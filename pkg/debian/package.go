package debian

// PackageInfo holds the metadata for a debian package
type PackageInfo struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Component    string `json:"component"`
	Suite        string `json:"suite"`
	Pocket       string `json:"pocket"`
	Architecture string `json:"architecture"`
}
