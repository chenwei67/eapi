package router

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"net/http"
)

type TestRequest struct {
	Name string `json:"name" xml:"name" yaml:"name"`
	Age  int    `json:"age" xml:"age" yaml:"age"`
}

// TestMustBindWith 测试 MustBindWith 方法
func TestMustBindWith(c *gin.Context) {
	var req TestRequest
	c.MustBindWith(&req, binding.JSON)
	c.JSON(http.StatusOK, gin.H{"message": "MustBindWith JSON success", "data": req})
}

// TestMustBindWithXML 测试 MustBindWith XML
func TestMustBindWithXML(c *gin.Context) {
	var req TestRequest
	c.MustBindWith(&req, binding.XML)
	c.JSON(http.StatusOK, gin.H{"message": "MustBindWith XML success", "data": req})
}

// TestShouldBindWith 测试 ShouldBindWith 方法
func TestShouldBindWith(c *gin.Context) {
	var req TestRequest
	if err := c.ShouldBindWith(&req, binding.YAML); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ShouldBindWith YAML success", "data": req})
}

// TestBindWith 测试 BindWith 方法
func TestBindWith(c *gin.Context) {
	var req TestRequest
	if err := c.BindWith(&req, binding.Form); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "BindWith Form success", "data": req})
}

// TestMustBindQuery 测试 MustBindQuery 方法
func TestMustBindQuery(c *gin.Context) {
	var req TestRequest
	c.MustBindWith(&req)
	c.JSON(http.StatusOK, gin.H{"message": "MustBindQuery success", "data": req})
}

// TestMustBindJSON 测试 MustBindJSON 方法
func TestMustBindJSON(c *gin.Context) {
	var req TestRequest
	c.MustBindWith(&req)
	c.JSON(http.StatusOK, gin.H{"message": "MustBindJSON success", "data": req})
}

func setupBindTestRoutes(r *gin.Engine) {
	r.POST("/test/mustbindwith", TestMustBindWith)
	r.POST("/test/mustbindwith-xml", TestMustBindWithXML)
	r.POST("/test/shouldbindwith", TestShouldBindWith)
	r.POST("/test/bindwith", TestBindWith)
	r.POST("/test/mustbindquery", TestMustBindQuery)
	r.POST("/test/mustbindjson", TestMustBindJSON)
}