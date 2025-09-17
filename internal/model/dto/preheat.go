package dto

type CommitHfSha struct {
	PipelineTag  string   `json:"pipeline_tag"`
	Tags         []string `json:"tags"`
	Sha          string   `json:"sha"`
	Likes        int      `json:"likes"`
	Downloads    int      `json:"downloads"`
	LastModified string   `json:"lastModified"`
	Siblings     []struct {
		Rfilename string `json:"rfilename"`
	} `json:"siblings"`
	UsedStorage int64 `json:"usedStorage"`
}
