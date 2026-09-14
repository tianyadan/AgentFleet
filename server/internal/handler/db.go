package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DBDatabases 列出可查询的库(数据查询项目)。
func (h *Handler) DBDatabases(c *gin.Context) {
	if h.svc.DBQuery == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data query not configured"})
		return
	}
	dbs, err := h.svc.DBQuery.Databases(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": dbs})
}

// DBTables 列出某库的表。
func (h *Handler) DBTables(c *gin.Context) {
	if h.svc.DBQuery == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data query not configured"})
		return
	}
	db := c.Query("database")
	if db == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "database param required"})
		return
	}
	tbls, err := h.svc.DBQuery.Tables(c.Request.Context(), db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": tbls})
}

// DBQuery 执行只读查询。body: {database, sql}
func (h *Handler) DBQuery(c *gin.Context) {
	if h.svc.DBQuery == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data query not configured"})
		return
	}
	var body struct {
		Database string `json:"database"`
		SQL      string `json:"sql"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.SQL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sql required"})
		return
	}
	// 限定库:前置 USE；库名去反引号后再校验,避免绕过白名单。
	fullSQL := body.SQL
	if body.Database != "" {
		d := strings.ReplaceAll(body.Database, "`", "")
		if !isAllowedDB(d) {
			c.JSON(http.StatusForbidden, gin.H{"error": "database not allowed"})
			return
		}
		fullSQL = "USE `" + d + "`; " + body.SQL
	}
	cols, rows, err := h.svc.DBQuery.Query(c.Request.Context(), fullSQL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"columns": cols, "rows": rows, "count": len(rows)})
}

func isAllowedDB(d string) bool {
	return d == "data-link" || d == "kfi-cloud"
}
