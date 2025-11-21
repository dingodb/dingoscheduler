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

type CreateCacheJobReq struct {
	Type         int32  `json:"type"`
	AidcCode     string `json:"aidcCode"`
	InstanceId   string `json:"instanceId"`
	Datatype     string `json:"datatype"`
	OrgRepo      string `json:"orgRepo"`
	Org          string `json:"org"`
	Repo         string `json:"repo"`
	RepositoryId int64  `json:"repositoryId"`
}

type CacheJobQuery struct {
	Id             int64  `json:"id"`
	Type           int32  `json:"type"`
	InstanceId     string `json:"instanceId"`
	Datatype       string `json:"datatype"`
	Org            string `json:"org"`
	Repo           string `json:"repo"`
	Page, PageSize int
}

type ResumeCacheJobReq struct {
	Id          int64  `json:"id"`
	Type        int32  `json:"type"`
	AidcCode    string `json:"aidcCode"`
	InstanceId  string `json:"instanceId"`
	Datatype    string `json:"datatype"`
	Org         string `json:"org"`
	Repo        string `json:"repo"`
	UsedStorage int64  `json:"usedStorage"`
}

type JobStatusReq struct {
	Id         int64  `json:"id"`
	AidcCode   string `json:"aidcCode"`
	InstanceId string `json:"instanceId"`
}

type RealtimeReq struct {
	CacheJobIds []int64 `json:"cacheJobIds"`
}

type RealtimeResp struct {
	CacheJobId   int64   `json:"cacheJobId"`
	StockSpeed   string  `json:"stockSpeed"`
	StockProcess float32 `json:"stockProcess"`
}

type UpdateJobStatusReq struct {
	Id         int64   `json:"id"`
	InstanceId string  `json:"instanceId"`
	Status     int32   `json:"status"`
	ErrorMsg   string  `json:"errorMsg"`
	Org        string  `json:"org"`
	Repo       string  `json:"repo"`
	Process    float32 `json:"process"`
}

type UpdateMountStatusReq struct {
	Id       int64  `json:"id"`
	Status   int32  `json:"status"`
	ErrorMsg string `json:"errorMsg"`
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

type PersistRepoReq struct {
	InstanceIds []string `json:"instanceIds"`
	Org         string   `json:"org"`
	Repo        string   `json:"repo"`
	OffVerify   bool     `json:"offVerify"`
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
	Status            string `json:"status"`
}

type RepositoryReq struct {
	Id int64 `json:"id"`
}

type TagQuery struct {
	Id       string
	Types    []string
	SubTypes []string
	Labels   []string
	DataType string
}

type MainTagQuery struct {
	Dataset string
}
