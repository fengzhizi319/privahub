// Command openapi-gen generates the OpenAPI 3 document served at
// /api/v1alpha1/docs (internal/controller/http/apidocs/openapi.json).
//
// It builds the real gin routers (main + inner port) on a throw-away SQLite
// database, walks every /api route, locates the handler function in the
// type-checked sources (golang.org/x/tools/go/packages) and derives:
//   - the request body schema from the variable passed to a bind call
//     (ShouldBindJSON / ShouldBind / bind(c, &req) ...);
//   - the response data schema from the value passed to response.OK(c, v).
//
// Usage:
//
//	CGO_ENABLED=1 go run ./cmd/openapi-gen -out internal/controller/http/apidocs/openapi.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/tools/go/packages"

	controllerhttp "github.com/fengzhizi319/privahub/internal/controller/http"
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao"
	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/wire"
	"github.com/fengzhizi319/privahub/pkg/config"
	"github.com/gin-gonic/gin"
)

const modulePath = "github.com/fengzhizi319/privahub"

type route struct {
	Method, Path, Handler string
	Inner                 bool
}

func main() {
	out := flag.String("out", "internal/controller/http/apidocs/openapi.json", "output file")
	flag.Parse()

	routes, err := collectRoutes()
	if err != nil {
		fatal(err)
	}
	g, err := newGenerator()
	if err != nil {
		fatal(err)
	}
	doc := g.document(routes)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d operations)\n", *out, len(routes))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "openapi-gen:", err)
	os.Exit(1)
}

// collectRoutes builds the application routers and returns every /api route.
func collectRoutes() ([]route, error) {
	dir, err := os.MkdirTemp("", "openapi-gen")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	_ = os.Setenv("PRIVAHUB_PROFILE", "test")
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = "file:" + filepath.Join(dir, "gen.db")
	cfg.Kuscia.PoolFile = filepath.Join(dir, "nodes.json")
	cfg.Auth.InitialPasswordFile = filepath.Join(dir, "initial-password")
	cfg.Server.APIDocs.Enabled = true
	cfg.Audit.SpoolDir = filepath.Join(dir, "audit-spool")
	log := zap.NewNop()
	db, err := dao.NewDB(&cfg.Database, log)
	if err != nil {
		return nil, err
	}
	if err := migration.AutoMigrate(db, log); err != nil {
		return nil, err
	}
	app := wire.NewApp(db, cfg, log)
	defer app.Shutdown()
	gin.SetMode(gin.ReleaseMode)
	var out []route
	add := func(e *gin.Engine, inner bool) {
		for _, ri := range e.Routes() {
			if strings.HasPrefix(ri.Path, "/api/") {
				out = append(out, route{Method: ri.Method, Path: ri.Path, Handler: ri.Handler, Inner: inner})
			}
		}
	}
	add(controllerhttp.NewRouter(log, app), false)
	add(controllerhttp.NewInnerRouter(log, app), true)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out, nil
}

type generator struct {
	pkgs    map[string]*packages.Package
	schemas map[string]any
	names   map[types.Type]string
}

func newGenerator() (*generator, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
		packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps}
	pkgs, err := packages.Load(cfg, "./internal/controller/http/...")
	if err != nil {
		return nil, err
	}
	g := &generator{pkgs: map[string]*packages.Package{}, schemas: map[string]any{}, names: map[types.Type]string{}}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, fmt.Errorf("load %s: %v", p.PkgPath, p.Errors[0])
		}
		g.pkgs[p.PkgPath] = p
	}
	return g, nil
}

var (
	methodRe  = regexp.MustCompile(`^(.+)\.\(\*?(\w+)\)\.(\w+)(-fm)?$`)
	closureRe = regexp.MustCompile(`^(.+)\.(\w+)((?:\.func\d+)+)$`)
	funcRe    = regexp.MustCompile(`^(.+)\.(\w+)$`)
)

// findHandler resolves a gin handler name to its function body.
func (g *generator) findHandler(name string) (*packages.Package, ast.Node) {
	if m := methodRe.FindStringSubmatch(name); m != nil {
		p := g.pkgs[m[1]]
		if p == nil {
			return nil, nil
		}
		for _, f := range p.Syntax {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Recv == nil || fd.Name.Name != m[3] || len(fd.Recv.List) == 0 {
					continue
				}
				if recvName(fd.Recv.List[0].Type) == m[2] {
					return p, fd
				}
			}
		}
		return p, nil
	}
	if m := closureRe.FindStringSubmatch(name); m != nil {
		p := g.pkgs[m[1]]
		if p == nil {
			return nil, nil
		}
		fd := findFunc(p, m[2])
		if fd == nil {
			return p, nil
		}
		var node ast.Node = fd
		for _, part := range strings.Split(strings.TrimPrefix(m[3], "."), ".") {
			n, _ := strconv.Atoi(strings.TrimPrefix(part, "func"))
			lit := nthFuncLit(node, n)
			if lit == nil {
				return p, nil
			}
			node = lit
		}
		return p, node
	}
	if m := funcRe.FindStringSubmatch(name); m != nil {
		if p := g.pkgs[m[1]]; p != nil {
			return p, findFunc(p, m[2])
		}
	}
	return nil, nil
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return recvName(t.X)
	}
	return ""
}

func findFunc(p *packages.Package, name string) *ast.FuncDecl {
	for _, f := range p.Syntax {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == name {
				return fd
			}
		}
	}
	return nil
}

// nthFuncLit returns the n-th (1-based) function literal directly nested in
// node, in source order (the Go compiler's funcN numbering).
func nthFuncLit(node ast.Node, n int) *ast.FuncLit {
	var body *ast.BlockStmt
	switch f := node.(type) {
	case *ast.FuncDecl:
		body = f.Body
	case *ast.FuncLit:
		body = f.Body
	}
	if body == nil {
		return nil
	}
	i := 0
	var found *ast.FuncLit
	ast.Inspect(body, func(x ast.Node) bool {
		if found != nil {
			return false
		}
		if fl, ok := x.(*ast.FuncLit); ok {
			i++
			if i == n {
				found = fl
			}
			return false // nested literals are numbered under their parent
		}
		return true
	})
	return found
}

type handlerInfo struct {
	Req       types.Type
	Resp      types.Type
	Multipart bool
	Doc       string
}

func (g *generator) analyze(p *packages.Package, node ast.Node) handlerInfo {
	var hi handlerInfo
	if node == nil || p == nil {
		return hi
	}
	if fd, ok := node.(*ast.FuncDecl); ok && fd.Doc != nil {
		hi.Doc = strings.TrimSpace(strings.Split(fd.Doc.Text(), "\n\n")[0])
	}
	ast.Inspect(node, func(x ast.Node) bool {
		call, ok := x.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := callName(call)
		lname := strings.ToLower(name)
		switch {
		case name == "FormFile" || name == "MultipartForm" || name == "PostForm":
			hi.Multipart = true
		case hi.Req == nil && strings.Contains(lname, "bind"):
			for _, a := range call.Args {
				if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.AND {
					if t := p.TypesInfo.TypeOf(u.X); t != nil {
						hi.Req = t
					}
				}
			}
		case hi.Resp == nil && name == "OK" && isResponsePkg(p, call) && len(call.Args) == 2:
			if t := p.TypesInfo.TypeOf(call.Args[1]); t != nil && !isNil(t) {
				hi.Resp = t
			}
		}
		return true
	})
	return hi
}

func callName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.Ident:
		return f.Name
	}
	return ""
}

func isResponsePkg(p *packages.Package, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	pn, ok := p.TypesInfo.Uses[id].(*types.PkgName)
	return ok && pn.Imported().Path() == modulePath+"/pkg/response"
}

func isNil(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.UntypedNil
}

func (g *generator) document(routes []route) map[string]any {
	paths := map[string]any{}
	for _, r := range routes {
		p, node := g.findHandler(r.Handler)
		hi := g.analyze(p, node)
		op := map[string]any{
			"operationId": operationID(r),
			"tags":        []string{tag(r.Path)},
			"responses": map[string]any{
				"200": map[string]any{
					"description": "SecretPad response envelope (business errors use status.code != 0 with HTTP 200)",
					"content": map[string]any{"application/json": map[string]any{
						"schema": envelope(g.schema(hi.Resp)),
					}},
				},
			},
			"x-handler": r.Handler,
		}
		if hi.Doc != "" {
			op["summary"] = firstLine(hi.Doc)
		}
		if code := middleware.APIResourceCodes[r.Path]; code != "" {
			op["x-api-resource"] = code
		}
		switch {
		case middleware.PublicPaths[r.Path]:
			op["security"] = []any{}
		case r.Inner:
			op["x-inner-port"] = true
			op["security"] = []any{map[string]any{"kusciaOrigin": []string{}}}
		case middleware.AdminOnlyPaths[r.Path]:
			op["x-admin-only"] = true
		case middleware.AuditorPaths[r.Path]:
			op["x-roles"] = []string{"AUDITOR", "ADMIN"}
		}
		if r.Method == "POST" || r.Method == "PUT" {
			ct := "application/json"
			if hi.Multipart {
				ct = "multipart/form-data"
			}
			var s any = map[string]any{"type": "object"}
			if hi.Req != nil {
				s = g.schema(hi.Req)
			}
			op["requestBody"] = map[string]any{"content": map[string]any{ct: map[string]any{"schema": s}}}
		}
		item, _ := paths[r.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[r.Path] = item
		}
		m := strings.ToLower(r.Method)
		if prev, ok := item[m].(map[string]any); ok {
			// Same path on both ports (e.g. user/node/resetPassword): keep
			// the main-port operation and flag the inner-port twin.
			prev["x-also-inner-port"] = true
			continue
		}
		item[m] = op
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Privahub API",
			"version": "v1alpha1",
			"description": "Go port of the SecretPad REST API. Generated by cmd/openapi-gen from the live route table. " +
				"Property names are the Go json tags; the server additionally accepts and returns camelCase " +
				"twins of snake_case keys (CaseKeysRequest/CaseKeysResponse middleware).",
		},
		"servers":  []any{map[string]any{"url": "/"}},
		"security": []any{map[string]any{"userToken": []string{}}},
		"paths":    paths,
		"components": map[string]any{
			"schemas": g.schemas,
			"securitySchemes": map[string]any{
				"userToken":    map[string]any{"type": "apiKey", "in": "header", "name": "User-Token"},
				"kusciaOrigin": map[string]any{"type": "apiKey", "in": "header", "name": "kuscia-origin-source"},
			},
		},
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func operationID(r route) string {
	id := strings.TrimPrefix(r.Path, "/api/")
	id = strings.NewReplacer("/", "_", "-", "_", "*", "").Replace(id)
	if r.Inner {
		id = "inner_" + id
	}
	return strings.ToLower(r.Method) + "_" + id
}

func tag(path string) string {
	parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(path, "/api/v1alpha1/"), "/api/"), "/")
	if parts[0] == "p2p" && len(parts) > 1 {
		return "p2p/" + parts[1]
	}
	return parts[0]
}

func envelope(data any) map[string]any {
	props := map[string]any{
		"status": map[string]any{"type": "object", "properties": map[string]any{
			"code": map[string]any{"type": "integer"},
			"msg":  map[string]any{"type": "string"},
		}},
	}
	if data != nil {
		props["data"] = data
	}
	return map[string]any{"type": "object", "properties": props}
}

// schema converts a Go type into an OpenAPI schema; named structs become
// components referenced by $ref.
func (g *generator) schema(t types.Type) any {
	if t == nil {
		return nil
	}
	switch tt := t.(type) {
	case *types.Pointer:
		return g.schema(tt.Elem())
	case *types.Alias:
		return g.schema(types.Unalias(tt))
	case *types.Named:
		obj := tt.Obj()
		if obj.Pkg() != nil {
			switch obj.Pkg().Path() + "." + obj.Name() {
			case "time.Time":
				return map[string]any{"type": "string", "format": "date-time"}
			case "time.Duration":
				return map[string]any{"type": "integer", "format": "int64"}
			case "encoding/json.RawMessage":
				return map[string]any{}
			case "github.com/gin-gonic/gin.H":
				return map[string]any{"type": "object", "additionalProperties": true}
			case "database/sql.NullString":
				return map[string]any{"type": "string", "nullable": true}
			case "gorm.io/gorm.DeletedAt", "database/sql.NullTime":
				return map[string]any{"type": "string", "format": "date-time", "nullable": true}
			}
		}
		st, ok := tt.Underlying().(*types.Struct)
		if !ok {
			return g.schema(tt.Underlying())
		}
		name, ok := g.names[tt]
		if !ok {
			name = g.componentName(tt)
			g.names[tt] = name
			g.schemas[name] = map[string]any{} // recursion guard
			g.schemas[name] = g.structSchema(st)
		}
		return map[string]any{"$ref": "#/components/schemas/" + name}
	case *types.Struct:
		return g.structSchema(tt)
	case *types.Slice:
		if b, ok := tt.Elem().(*types.Basic); ok && b.Kind() == types.Byte {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": orAny(g.schema(tt.Elem()))}
	case *types.Array:
		return map[string]any{"type": "array", "items": orAny(g.schema(tt.Elem()))}
	case *types.Map:
		return map[string]any{"type": "object", "additionalProperties": orAny(g.schema(tt.Elem()))}
	case *types.Interface:
		return map[string]any{}
	case *types.Basic:
		switch {
		case tt.Info()&types.IsBoolean != 0:
			return map[string]any{"type": "boolean"}
		case tt.Info()&types.IsInteger != 0:
			if tt.Kind() == types.Int64 || tt.Kind() == types.Uint64 {
				return map[string]any{"type": "integer", "format": "int64"}
			}
			return map[string]any{"type": "integer"}
		case tt.Info()&types.IsFloat != 0:
			return map[string]any{"type": "number"}
		case tt.Info()&types.IsString != 0:
			return map[string]any{"type": "string"}
		}
	}
	return map[string]any{}
}

func orAny(s any) any {
	if s == nil {
		return map[string]any{}
	}
	return s
}

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9_.]+`)

func (g *generator) componentName(t *types.Named) string {
	pkg := "builtin"
	if t.Obj().Pkg() != nil {
		pkg = t.Obj().Pkg().Name()
	}
	name := pkg + "." + t.Obj().Name()
	if args := t.TypeArgs(); args != nil && args.Len() > 0 {
		var parts []string
		for i := 0; i < args.Len(); i++ {
			parts = append(parts, types.TypeString(args.At(i), func(p *types.Package) string { return p.Name() }))
		}
		name += "_" + strings.Join(parts, "_")
	}
	name = nonIdent.ReplaceAllString(name, "_")
	base, i := name, 2
	for {
		if _, taken := g.schemas[name]; !taken {
			return name
		}
		name = fmt.Sprintf("%s%d", base, i)
		i++
	}
}

func (g *generator) structSchema(st *types.Struct) map[string]any {
	props := map[string]any{}
	var required []string
	g.addFields(st, props, &required)
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		s["required"] = required
	}
	return s
}

func (g *generator) addFields(st *types.Struct, props map[string]any, required *[]string) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i))
		jsonTag := tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		name := strings.Split(jsonTag, ",")[0]
		if f.Embedded() && name == "" {
			ft := f.Type()
			if p, ok := ft.(*types.Pointer); ok {
				ft = p.Elem()
			}
			if es, ok := ft.Underlying().(*types.Struct); ok {
				g.addFields(es, props, required)
				continue
			}
		}
		if !f.Exported() {
			continue
		}
		if name == "" {
			if form := strings.Split(tag.Get("form"), ",")[0]; form != "" && form != "-" {
				name = form
			} else {
				name = f.Name()
			}
		}
		if _, dup := props[name]; dup {
			continue
		}
		props[name] = orAny(g.schema(f.Type()))
		if strings.Contains(tag.Get("binding"), "required") {
			*required = append(*required, name)
		}
	}
}
