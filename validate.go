package prettyview

import "github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"

// This file is v2 live parse-status feedback (issue #45). The tolerant parsers emit a
// KindError marker line wherever they recover from malformed input, so the validity of
// the last structured reformat is available at no extra parse cost. ParseStatus exposes
// it; OnValidationChanged signals valid<->invalid transitions. It is read-only feedback
// and never touches the caret.

// ParseStatus is the validity of the most recent structured reformat (Reformat /
// format-on-pause). OK is true when the structured parse recovered no errors; otherwise
// ErrorLine is the first error marker's display-line index (it is -1 when OK).
type ParseStatus struct {
	OK        bool
	ErrorLine int
}

// ParseStatus returns the validity of the last structured reformat. A widget that has
// only ever shown the raw edit projection (no reformat yet) reports OK.
func (pv *PrettyView) ParseStatus() ParseStatus { return pv.parseStatus }

// SetOnValidationChanged registers a callback fired (on the Fyne goroutine) whenever the
// structured parse flips between valid and invalid. Setting it replaces any previous
// callback. Only meaningful for editable widgets.
func (pv *PrettyView) SetOnValidationChanged(fn func(ParseStatus)) { pv.onValidation = fn }

// setParseStatus records a freshly-computed status and fires OnValidationChanged when
// the valid/invalid state flipped.
func (pv *PrettyView) setParseStatus(st ParseStatus) {
	flipped := st.OK != pv.parseStatus.OK
	pv.parseStatus = st
	if flipped && pv.onValidation != nil {
		pv.onValidation(st)
	}
}

// parseStatusOf scans a structured document for the first recovered-error marker line.
func parseStatusOf(d *model.Document) ParseStatus {
	for li := range d.Lines {
		o := d.Lines[li].Owner
		if o != model.NoNode && int(o) < len(d.Nodes) && d.Nodes[o].Kind == model.KindError {
			return ParseStatus{OK: false, ErrorLine: li}
		}
	}
	return ParseStatus{OK: true, ErrorLine: -1}
}

// lineIsError reports whether display line li is a recovered-error marker (drives the
// gutter tint). Cheap O(1) — it reads the line's owning node's kind.
func (pv *PrettyView) lineIsError(li int32) bool {
	if pv.doc == nil || int(li) < 0 || int(li) >= len(pv.doc.Lines) {
		return false
	}
	o := pv.doc.Lines[li].Owner
	return o != model.NoNode && int(o) < len(pv.doc.Nodes) && pv.doc.Nodes[o].Kind == model.KindError
}
