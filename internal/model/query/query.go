package query

type ModelFileRecordQuery struct {
	InstanceId string
	Datatype   string
	Org        string
	Repo       string
	FileName   string
	Etag       string
	StartPos   int64
}

type PreheatJobQuery struct {
	InstanceId string `json:"instanceId"`
	Datatype   string `json:"datatype"`
	Org        string `json:"org"`
	Repo       string `json:"repo"`
	Token      string `json:"token"`
}

type PathInfoQuery struct {
	Datatype  string   `json:"datatype"`
	Org       string   `json:"org"`
	Repo      string   `json:"repo"`
	Revision  string   `json:"revision"`
	Token     string   `json:"token"`
	FileNames []string `json:"fileNames"`
}

type PersistRepoQuery struct {
	InstanceIds []string `json:"instanceIds"`
	Token       string   `json:"token"`
}

type ModelQuery struct {
	InstanceId        string `json:"instanceId"`
	Name              string
	Page              int
	PageSize          int
	Sort              string
	Order             string
	PipelineTag       string
	Library           string
	Apps              string
	InferenceProvider string
	Language          string
	License           string
	Other             string
}

type TagQuery struct {
	Id       string
	Types    []string
	SubTypes []string
	Labels   []string
}
