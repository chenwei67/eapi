package shop

import (
	"encoding/json"
	"net/http"

	"server/pkg/E"
	"server/pkg/handler"
	"server/pkg/view"

	"github.com/gin-gonic/gin"
)

// GoodsCreate 创建商品接口
// @summary 创建商品
// @consume application/json
// @produce application/json
func GoodsCreate(c *handler.CustomContext) {
	var req view.GoodsCreateReq
	if err := c.Bind(&req); err != nil {
		// 参数无效
		c.JSON(http.StatusBadRequest, view.ErrInvalidArgument)
		return
	}

	// Biz logic here ...

	var res view.GoodsCreateRes
	// 创建成功
	c.JSONOK(res)
}

// GoodsDown 下架商品
func GoodsDown(c *gin.Context) {
	// 商品 GUID
	_ = c.Param("guid")
	// 操作人 UID
	_ = c.PostForm("operatorUid")
	// 日期范围
	_, _ = c.GetPostFormArray("dateRange")
	// Default Query
	_ = c.DefaultQuery("defaultQuery", "xxxx")
	// Default Post Form
	_ = c.DefaultPostForm("defaultPostForm", "yyyy")

	c.XML(http.StatusOK, view.GoodsDownRes{})
}

type GoodsInfoPathParams struct {
	// Goods Guid
	Guid int `uri:"guid"`
}

// GoodsInfo 商品详情
// @consume application/json
// @produce application/json
func GoodsInfo(c *gin.Context) {
	var params GoodsInfoPathParams
	_ = c.BindUri(&params)

	c.JSON(http.StatusOK, view.GoodsInfoRes{})
}

// GoodsDelete 删除商品
// @consume multipart/form-data
// @tags High Priority Tag
// @security oauth2 goods:write
func GoodsDelete(c *handler.CustomContext) {
	var request view.GoodsDeleteRequest
	_ = c.Bind(&request)
}

// WrappedHandler
// @deprecated
func WrappedHandler(c *handler.CustomContext) {
	_ = c.Query("hello")
	_ = c.Query("world")
	if false {
		c.JSON(http.StatusBadRequest, json.RawMessage("{\"hello\": \"world\"}"))
	}

	// 自定义响应函数
	c.JSONOK(map[string]interface{}{})
}

// TestESuccess 测试E.Success包级别函数
// @summary 测试E.Success响应
// @consume application/json
// @produce application/json
// @tags Shop
func TestESuccess(c *gin.Context) {
	var res view.GoodsInfoRes
	// 使用E.Success包级别函数
	c.JSON(http.StatusOK, E.Success(res))
}
