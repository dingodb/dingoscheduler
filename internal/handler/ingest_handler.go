package handler

import (
	pb "dingoscheduler/pkg/proto/manager"
	"dingoscheduler/pkg/util"
	"github.com/labstack/echo/v4"
	"net/http"
)

func (h *ManagerHandler) IngestRepository(c echo.Context) error {
	var req struct {
		InstanceID string `json:"instanceId"`
		Namespace  string `json:"namespace"`
		RepoType   string `json:"repoType"`
		Repo       string `json:"repo"`
		Revision   string `json:"revision"`
		Commit     string `json:"commit"`
		Online     *bool  `json:"online"`
	}
	if err := c.Bind(&req); err != nil {
		return util.ErrorRequestParamCN(c)
	}
	online := true
	if req.Online != nil {
		online = *req.Online
	}
	result, err := h.schedulerService.IngestRepository(c.Request().Context(), &pb.IngestRepositoryRequest{InstanceId: req.InstanceID, Namespace: req.Namespace, RepoType: req.RepoType, Repo: req.Repo, Revision: req.Revision, Commit: req.Commit, Online: online})
	if err != nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error(), "code": "INGEST_NOT_COMMITTED"})
	}
	return util.NormalResponseData(c, map[string]interface{}{"repositoryId": result.RepositoryId, "namespace": req.Namespace, "repoType": req.RepoType, "repo": req.Repo, "revision": req.Revision, "commit": result.Commit, "fileCount": result.FileCount, "usedStorage": result.UsedStorage, "status": "persisted"})
}

func (h *ManagerHandler) UploadedHoldings(c echo.Context) error {
	rows, err := h.schedulerService.UploadedHoldings(c.Request().Context(), c.QueryParam("instanceId"), c.QueryParam("namespace"), c.QueryParam("repoType"), c.QueryParam("repo"), c.QueryParam("path"), c.QueryParam("sha256"))
	if err != nil {
		return util.ResponseError(c, err)
	}
	return util.NormalResponseData(c, map[string]interface{}{"items": rows, "count": len(rows)})
}

func (h *ManagerHandler) UploadedNodeHoldings(c echo.Context) error {
	rows, err := h.schedulerService.UploadedHoldings(c.Request().Context(), c.Param("instanceId"), c.QueryParam("namespace"), c.QueryParam("repoType"), c.QueryParam("repo"), c.QueryParam("path"), c.QueryParam("sha256"))
	if err != nil {
		return util.ResponseError(c, err)
	}
	state, err := h.schedulerService.UploadedNodeInventoryState(c.Request().Context(), c.Param("instanceId"))
	if err != nil {
		return util.ResponseError(c, err)
	}
	return util.NormalResponseData(c, map[string]interface{}{"instanceId": c.Param("instanceId"), "state": state, "items": rows, "count": len(rows)})
}

func (h *ManagerHandler) UploadedRepositories(c echo.Context) error {
	rows, err := h.schedulerService.UploadedRepositories(c.Request().Context())
	if err != nil {
		return util.ResponseError(c, err)
	}
	return util.NormalResponseData(c, map[string]interface{}{"items": rows, "count": len(rows)})
}
