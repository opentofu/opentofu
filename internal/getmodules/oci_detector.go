package getmodules

type OciDetector struct{}

func (d *OciDetector) Detect(src, pwd string) (string, bool, error) {
	if len(src) == 0 {
		return "", false, nil
	}
	return "", false, nil
}
