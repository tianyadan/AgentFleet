package dbquery

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// AllowedDBs 可查询的业务库(远端)。
var AllowedDBs = map[string]bool{
	"data-link": true,
	"kfi-cloud": true,
}

// readOnlyRe 只允许只读语句(可带可选的 USE 前缀/多语句中的只读语句)。
var readOnlyRe = regexp.MustCompile(`(?i)^(USE\s+\S+\s*;\s*)?(SELECT|SHOW|DESC|DESCRIBE|EXPLAIN)\b`)

// Client 封装对远端只读库的访问(连接用户 tian-ai)。
type Client struct {
	db *sql.DB
}

// New 建立连接(不指定默认库,跨库查询)。
func New(dsn string) (*Client, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	return &Client{db: db}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Databases 列出可查询的库(仅允许列表内的)。
func (c *Client) Databases(ctx context.Context) ([]string, error) {
	rows, err := c.db.QueryContext(ctx, `SHOW DATABASES`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var db string
		if err := rows.Scan(&db); err != nil {
			return nil, err
		}
		if AllowedDBs[db] {
			out = append(out, db)
		}
	}
	return out, rows.Err()
}

// Tables 列出某库的表。
func (c *Client) Tables(ctx context.Context, dbName string) ([]string, error) {
	if !AllowedDBs[dbName] {
		return nil, fmt.Errorf("database not allowed: %s", dbName)
	}
	dbName = strings.ReplaceAll(dbName, "`", "")
	rows, err := c.db.QueryContext(ctx, fmt.Sprintf("SHOW TABLES FROM `%s`", dbName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TableSchema 返回某库某表的列结构。
func (c *Client) TableSchema(ctx context.Context, dbName, table string) ([]map[string]interface{}, error) {
	if !AllowedDBs[dbName] {
		return nil, fmt.Errorf("database not allowed: %s", dbName)
	}
	dbName = strings.ReplaceAll(dbName, "`", "")
	table = strings.ReplaceAll(table, "`", "")
	rows, err := c.db.QueryContext(ctx, fmt.Sprintf("DESC `%s`.`%s`", dbName, table))
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

// Query 只读查询: 仅允许 readOnlyRe 开头的语句,返回行列。
func (c *Client) Query(ctx context.Context, sqlStr string) (cols []string, rowsOut []map[string]interface{}, err error) {
	s := strings.TrimSpace(sqlStr)
	if !readOnlyRe.MatchString(s) {
		return nil, nil, fmt.Errorf("only read-only queries allowed")
	}
	rows, err := c.db.QueryContext(ctx, s)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err = rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	colsOut := make([]string, len(cols))
	copy(colsOut, cols)
	// 行数上限保护
	limit := 200
	for rows.Next() && limit >= 0 {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := map[string]interface{}{}
		for i, cname := range cols {
			row[cname] = stringify(vals[i])
		}
		rowsOut = append(rowsOut, row)
		limit--
	}
	return colsOut, rowsOut, rows.Err()
}

func scanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]interface{}{}
		for i, cname := range cols {
			row[cname] = stringify(vals[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func stringify(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(t)
	case string:
		return t
	default:
		return fmt.Sprint(v)
	}
}

func (c *Client) Close() error { return c.db.Close() }
