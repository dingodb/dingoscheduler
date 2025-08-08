package dto

type CommitHfSha struct {
	Sha      string `json:"sha"`
	Siblings []struct {
		Rfilename string `json:"rfilename"`
	} `json:"siblings"`
}
