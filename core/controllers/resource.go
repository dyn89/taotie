package controllers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator"
	"math"
	"taotie/core/flog"
	"taotie/core/model"
	"taotie/core/util"
)

type ListResourceRequest struct {
	Id   int64    `json:"id"`
	Name string   `json:"name"`
	Url  string   `json:"url"`
	Sort []string `json:"sort"`
	PageHelp
}

type ListResourceResponse struct {
	Resources []model.Resource `json:"resources"`
	PageHelp
}

func ListResource(c *gin.Context) {
	resp := new(Resp)

	respResult := new(ListResourceResponse)
	req := new(ListResourceRequest)
	defer func() {
		JSONL(c, 200, req, resp)
	}()

	if errResp := ParseJSON(c, req); errResp != nil {
		resp.Error = errResp
		return
	}

	var validate = validator.New()
	err := validate.Struct(req)
	if err != nil {
		flog.Log.Errorf("ListResource err: %s", err.Error())
		resp.Error = Error(ParasError, err.Error())
		return
	}

	// new query list session
	session := model.Rdb.Client.NewSession()
	defer session.Close()

	// group list where prepare
	session.Table(new(model.Resource)).Where("1=1")

	// query prepare
	if req.Id != 0 {
		session.And("id=?", req.Id)
	}
	if req.Name != "" {
		session.And("name=?", req.Name)
	}

	session.And("admin=?", 1)

	if req.Url != "" {
		urlHash, _ := util.Sha256([]byte(req.Url))
		session.And("url_hash=?", urlHash)
	}

	// count num
	countSession := session.Clone()
	defer countSession.Close()
	total, err := countSession.Count()
	if err != nil {
		flog.Log.Errorf("ListResource err:%s", err.Error())
		resp.Error = Error(DBError, err.Error())
		return
	}

	// if count>0 start list
	r := make([]model.Resource, 0)
	p := &req.PageHelp
	if total == 0 {
		if p.Limit == 0 {
			p.Limit = 20
		}
	} else {
		// sql build
		p.build(session, req.Sort, model.ResourceSortName)
		// do query
		err = session.Find(&r)
		if err != nil {
			flog.Log.Errorf("ListResource err:%s", err.Error())
			resp.Error = Error(DBError, err.Error())
			return
		}
	}

	// result
	respResult.Resources = r
	p.Pages = int(math.Ceil(float64(total) / float64(p.Limit)))
	p.Total = int(total)
	respResult.PageHelp = *p
	resp.Data = respResult
	resp.Flag = true
}

type AssignResourceToGroupRequest struct {
	GroupId         int64   `json:"group_id"`
	ResourceRelease int     `json:"resource_release"`
	Resources       []int64 `json:"resources"`
}

func AssignResourceToGroup(c *gin.Context) {
	resp := new(Resp)
	req := new(AssignResourceToGroupRequest)
	defer func() {
		JSONL(c, 200, req, resp)
	}()

	if errResp := ParseJSON(c, req); errResp != nil {
		resp.Error = errResp
		return
	}

	resourceNums := len(req.Resources)
	if resourceNums == 0 && req.ResourceRelease != 1 {
		flog.Log.Errorf("AssignGroupAndResource err:%s", "resources empty")
		resp.Error = Error(ParasError, "resources empty")
		return
	}

	if req.GroupId == 0 {
		flog.Log.Errorf("AssignGroupAndResource err:%s", "group id empty")
		resp.Error = Error(ParasError, "group_id")
		return
	}

	g := new(model.Group)
	g.Id = req.GroupId
	exist, err := g.GetById()
	if err != nil {
		flog.Log.Errorf("AssignGroupAndResource err:%s", err.Error())
		resp.Error = Error(DBError, err.Error())
		return
	}

	if !exist {
		flog.Log.Errorf("AssignGroupAndResource err:%s", "group not found")
		resp.Error = Error(GroupNotFound, "")
		return
	}

	if resourceNums > 0 {
		num, err := model.Rdb.Client.Table(new(model.Resource)).In("id", req.Resources).Count()
		if err != nil {
			flog.Log.Errorf("AssignGroupAndResource err:%s", err.Error())
			resp.Error = Error(DBError, err.Error())
			return
		}

		if int(num) != resourceNums {
			flog.Log.Errorf("AssignGroupAndResource err:%s", "resource wrong")
			resp.Error = Error(ResourceCountNumNotRight, fmt.Sprintf("resource wrong:%d!=%d", num, resourceNums))
			return
		}
	}

	session := model.Rdb.Client.NewSession()
	defer session.Close()

	err = session.Begin()
	if err != nil {
		flog.Log.Errorf("AssignGroupAndResource err:%s", err.Error())
		resp.Error = Error(DBError, err.Error())
		return
	}

	if len(req.Resources) > 0 {
		session.In("resource_id", req.Resources)
	}

	_, err = session.Where("group_id=?", req.GroupId).Delete(new(model.GroupResource))
	if err != nil {
		session.Rollback()
		flog.Log.Errorf("AssignGroupAndResource err:%s", err.Error())
		resp.Error = Error(DBError, err.Error())
		return
	}

	if req.ResourceRelease != 1 {
		rs := make([]model.GroupResource, 0, resourceNums)
		for _, r := range req.Resources {
			rs = append(rs, model.GroupResource{GroupId: req.GroupId, ResourceId: r})
		}
		_, err = session.Insert(rs)
		if err != nil {
			session.Rollback()
			flog.Log.Errorf("AssignGroupAndResource err:%s", err.Error())
			resp.Error = Error(DBError, err.Error())
			return
		}
	}

	err = session.Commit()
	if err != nil {
		session.Rollback()
		flog.Log.Errorf("AssignGroupAndResource err:%s", err.Error())
		resp.Error = Error(DBError, err.Error())
		return
	}
	resp.Flag = true
}
