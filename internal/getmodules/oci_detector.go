package getmodules

import "fmt"

type OciDetector struct{}

func (d *OciDetector) Detect(src, pwd string) (string, bool, error) {
	if len(src) == 0 {
		return "", false, nil
	}

	fmt.Println("-- OCI DEBUG:" + src + " -- " + pwd)
	return "", false, nil
}
