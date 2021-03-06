// @llr REQ-0-DDLN-SWL-014
// @llr REQ-0-DDLN-SWL-002
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/daedaleanai/reqtraq/git"
)

var (
	reCertdoc = regexp.MustCompile(`(\d+)-(\w+)-(\d+)-(\w+)`) // project number, project abbreviation, certdoc type number, certdoc type
	reStart   = regexp.MustCompile(`(?i)^\s*req:\s*$`)        // 'req:' standalone on a line
	reEnd     = regexp.MustCompile(`(?i)^\s*/req\s*$`)        // '/req' standalone on a line
)

// lyxState is the information needed to keep around on a stack to parse the
// nested inset/layout structure of a .lyx file
type lyxState struct {
	lineNo  int    // line on which this was found
	element string // layout/inset/preamble/etc
	arg     string // first token after the begin_layout or begin_inset
}

// a lyxStack keeps track of the \begin_  \end_ pairs
type lyxStack []lyxState

func (s *lyxStack) push(lno int, line, arg string) {
	element := strings.SplitN(line[len(`\begin_`):], " ", 2)[0]
	*s = append(*s, lyxState{lno, element, arg})
}
func (s *lyxStack) pop(lno int, line string) error {
	element := strings.SplitN(line[len(`\end_`):], " ", 2)[0]
	top := s.top()
	if top.element != element {
		return fmt.Errorf("lyx file malformed: begin %s line %d ended by end %s line %d", top.element, top.lineNo, element, lno)
	}
	if len(*s) > 0 {
		*s = (*s)[:len(*s)-1]
	}
	return nil
}
func (s lyxStack) top() lyxState {
	if len(s) > 0 {
		return s[len(s)-1]
	}
	return lyxState{}
}

// inNoteLayout returns true when the current state stack top is 'Layout' inside an 'inset Note'
func (s lyxStack) inNoteLayout() bool {
	size := len(s)
	if size < 2 {
		return false
	}
	return s[size-2].element == "inset" && s[size-2].arg == "Note" && s[size-1].element == "layout"
}

// ParseLyx reads a .lyx file finding blocks of text bracketed by
// notes containing "req:"  ...  "/req".
// It returns a slice of strings with one element per req:/req block
// containing the text in layout blocks, skipping (hopefully) the inset data.
// or an error describing a problem parsing the lines.
// It linkifies the lyx file and writes it to the provided writer.
func ParseLyx(f string, w io.Writer) ([]string, error) {
	var (
		reqs []string

		state         lyxStack
		preamblestart bool
		inreq         bool
		reqid         string
		aftertitle    bool
		reqstart      int
		reqbuf        bytes.Buffer
	)
	r, err := os.Open(f)
	if err != nil {
		return nil, err
	}
	scan := bufio.NewScanner(r)

	// Cache some info related to the git repo context.
	repo := git.RepoName()
	pathInRepo, err := git.PathInRepo(f)
	if err != nil {
		return nil, fmt.Errorf("File %s not found in repo.", f)
	}
	dirInRepo := filepath.Dir(pathInRepo)

	for lno := 1; scan.Scan(); lno++ {
		outline := scan.Text()
		line := outline
		istext := line != "" && !strings.HasPrefix(line, `\`) && !strings.HasPrefix(line, `#`)
		fields := strings.Fields(line)
		arg := ""
		if len(fields) > 1 {
			arg = fields[1]
		}
		switch {
		case strings.HasPrefix(line, `\textclass`):
			// Next is the preamble.
			preamblestart = true

		case preamblestart:
			preamblestart = false
			if strings.HasPrefix(line, `\begin_preamble`) {
				// The preable already exists.
				state.push(lno, line, "")
			} else {
				// There is no preamble, we add it ourselves.
				// ..if we want to.
			}

		case line == `\use_hyperref false`:
			// Required so the anchors end up in the PDF when converting.
			outline = `\use_hyperref true`

		case state.top().element == "preamble" && strings.HasPrefix(line, `\end_preamble`):
			if err = state.pop(lno, line); err != nil {
				return nil, err
			}

		case strings.HasPrefix(line, `\begin_layout`):
			state.push(lno, line, arg)
			if aftertitle {
				aftertitle = false
				outline = fmt.Sprintf(`%s
\begin_inset ERT
status open

\begin_layout Plain Layout


\backslash
hypertarget{%s}
\end_layout

\end_inset
`, outline, reqid)
			}

		case strings.HasPrefix(line, `\begin_inset`):
			state.push(lno, line, arg)

		case strings.HasPrefix(line, `\end_layout`):
			if err = state.pop(lno, line); err != nil {
				return nil, err
			}

		case strings.HasPrefix(line, `\end_inset`):
			if err = state.pop(lno, line); err != nil {
				return nil, err
			}

		case istext && state.inNoteLayout() && reStart.Match(scan.Bytes()):
			if inreq {
				return nil, fmt.Errorf("malformed requirement tag: 'req:' on line %d comes after previous unclosed one at line %d\n", lno, reqstart)
			}
			reqstart = lno
			inreq = true
			aftertitle = true

		case istext && inreq && state.inNoteLayout() && reEnd.Match(scan.Bytes()):
			if !inreq {
				return nil, fmt.Errorf("malformed requirement tag: '/req' on line %d has no corresponding opening req:\n", lno)
			}
			inreq = false
			reqs = append(reqs, reqbuf.String())
			reqbuf.Reset()

		case (istext || line == "") && inreq && state.top().element != "inset": // text layout content in a req bracketed block
			// an empty line means that a Lyx zparagraph has ended. simply append a \n to the previously parsed line and go to the next line
			if line == "" {
				reqbuf.WriteByte('\n')
				continue
			}
			isFirstLine := reqbuf.Len() == 0
			if isFirstLine {
				reqIDs := ReReqID.FindAllString(outline, -1)
				switch len(reqIDs) {
				case 0:
					return nil, fmt.Errorf("malformed requirement title: missing ID on line %d: %q", lno, outline)
				case 1:
					reqid = reqIDs[0]
				default:
					return nil, fmt.Errorf("malformed requirement title: too many IDs on line %d: %q", lno, outline)
				}
			} else {
				count := len(ReReqID.FindAllString(reqbuf.String(), -1))
				countCurrent := len(ReReqID.FindAllString(line, -1))
				r := reqbuf.String() + line
				indexes := ReReqID.FindAllStringIndex(r, -1)
				if count+countCurrent < len(indexes) {
					// There is a requirement ID which is split on two lines.
					// We move the entire requirement to the second line.
					reqbuf.Truncate(indexes[count][0])
					line = r[indexes[count][0]:] + line
				}
				if outline, err = linkify(outline, repo, dirInRepo); err != nil {
					return nil, fmt.Errorf("malformed requirement: cannot linkify ID on line %d: %q because: %s", lno, outline, err)
				}
			}

			reqbuf.WriteString(line)

		}
		if _, err := w.Write([]byte(outline)); err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return nil, err
		}
	}

	if err := scan.Err(); err != nil {
		return nil, err
	}

	return reqs, nil
}

var FileTypeToReqType = map[string]string{
	"ORD": "SYS",
	"SRD": "SWH",
	"HRD": "HWH",
	"SDD": "SWL",
	"HDD": "HWL"}

var docNamePerReqIDType = map[string]string{
	"SYS": "100-ORD",
	"SWH": "211-SRD",
	"SWL": "212-SDD",
	"HWH": "311-HRD",
	"HWL": "312-HDD",
}

var docNameConventions = map[string]string{
	"H":      "0",
	"DS":     "1",
	"SRS":    "6",
	"SDS":    "7",
	"SCS":    "8",
	"HRS":    "9",
	"HCS":    "10",
	"DAS":    "11",
	"HDS":    "12",
	"HVVS":   "13",
	"HAS":    "14",
	"HCMS":   "15",
	"TAS":    "34",
	"ORD":    "100",
	"SP":     "150",
	"SFA":    "151",
	"PSAC":   "200",
	"SCMP":   "201",
	"SQAP":   "202",
	"SDP":    "203",
	"SVP":    "204",
	"TQP":    "205",
	"SAS":    "206",
	"SRD":    "211",
	"SDD":    "212",
	"SVCP":   "213",
	"PHAK":   "300",
	"HRD":    "311",
	"HDD":    "312",
	"CLPSAC": "101",
	"CLSDP":  "102",
	"CLSVP":  "103",
	"CLSCMP": "104",
	"CLSQAP": "105",
	"CLSDD":  "107",
	"CLSRD":  "106",
	"CLSVCP": "108",
	"CLSCI":  "109",
	"CLTQP":  "110",
	"CLSAS":  "111",
	"TPPSAC": "201",
	"TPSRD":  "206",
	"TPSDD":  "207",
	"TPSVCP": "208",
	"TPHRD":  "209",
	"TPORD":  "210",
	"TPSFHA": "211",
	"TPFFPA": "212",
}

func linkify(s, repo, dirInRepo string) (string, error) {
	parmatch := ReReqID.FindAllStringSubmatchIndex(s, -1)
	var res bytes.Buffer
	parsedTo := 0
	for _, ids := range parmatch {
		// For example: ["REQ-0-DDLN-SYS-006" "0" "DDLN" "SYS" "006"]
		res.WriteString(s[parsedTo:ids[0]])
		reqID := s[ids[0]:ids[1]]
		parsedTo = ids[1]
		// As per REQ-0-DDLN-SWH-002:
		// REQ-[project/system number]-[project/system abbreviation]-[SSS or SWH or SWL or HWH or HWL]-[a unique alphanumeric sequence],
		numberAbbrev := s[ids[2]:ids[5]]
		reqType := s[ids[6]:ids[7]]
		if len(ids) != 10 {
			// This should not happen.
			return "", fmt.Errorf("regexp cannot be used, please file a bug in Devtools: %q", ids)
		}
		docType, ok := docNamePerReqIDType[reqType]
		if !ok {
			return "", fmt.Errorf("unknown requirement type: %q (in %q)", reqType, reqID)
		}
		// For example: 0-DDLN-0-ORD
		name := fmt.Sprintf("%s-%s", numberAbbrev, docType)
		url := fmt.Sprintf("http://a.daedalean.ai/docs/%s/%s/%s.pdf#%s", repo, dirInRepo, name, reqID)
		res.WriteString(fmt.Sprintf(`
\begin_inset CommandInset href
LatexCommand href
name "%s"
target "%s"

\end_inset

`, reqID, url))
	}
	res.WriteString(s[parsedTo:len(s)])
	return res.String(), nil
}
