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

type CacheJobQuery struct {
	Id             int64  `json:"id"`
	Type           int32  `json:"type"`
	InstanceId     string `json:"instanceId"`
	Datatype       string `json:"datatype"`
	Org            string `json:"org"`
	Repo           string `json:"repo"`
	Token          string `json:"token"`
	Page, PageSize int
}

type JobStatus struct {
	Id         int64  `json:"id"`
	InstanceId string `json:"instanceId"`
	Status     int32  `json:"status"`
	ErrorMsg   string `json:"errorMsg"`
}

type UpdateMountCachePidReq struct {
	Id  int64 `json:"id"`
	Pid int32 `json:"pid"`
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
	InstanceIds   []string `json:"instanceIds"`
	Authorization string   `json:"authorization"`
	Org           string   `json:"org"`
	Repo          string   `json:"repo"`
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
	Datatype          string `json:"datatype"`
}

type TagQuery struct {
	Id       string
	Types    []string
	SubTypes []string
	Labels   []string
}

type MainTagQuery struct {
	Dataset string
}
