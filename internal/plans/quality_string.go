// Code generated by "stringer -type Quality"; DO NOT EDIT.

package plans

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Errored-0]
	_ = x[NoChanges-1]
	_ = x[Concise-2]
}

const _Quality_name = "ErroredNoChangesConcise"

var _Quality_index = [...]uint8{0, 7, 16, 23}

func (i Quality) String() string {
	if i < 0 || i >= Quality(len(_Quality_index)-1) {
		return "Quality(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _Quality_name[_Quality_index[i]:_Quality_index[i+1]]
}
