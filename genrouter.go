package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	goSuffix = ".go"
)

func fileList(dir string) (fList []string) {
	curDir, err := os.Open(dir)
	defer curDir.Close()
	if err != nil {
		log.Fatal(err)
	}
	files, err := curDir.Readdir(0)
	if err != nil {
		log.Fatal(err)
	}
	for _, file := range files {
		if file.IsDir() {
			fList = append(fList, fileList(dir+string(os.PathSeparator)+file.Name())...)
		}
		if strings.HasSuffix(file.Name(), goSuffix) {
			fList = append(fList, dir+string(os.PathSeparator)+file.Name())
		}
	}
	return fList
}

const (
	requestCtx   = "RequestCtx"
	prefixMethod = "//@METHOD:"
	prefixPath   = "//@PATH:"
)

func filterFunc(decl *ast.FuncDecl) bool {
	if !decl.Name.IsExported() {
		return false
	}
	if decl.Type.Results.NumFields() != 0 {
		return false
	}
	if decl.Doc == nil {
		return false
	}
	if decl.Recv.NumFields() != 0 {
		return false
	}
	if decl.Type.Params.NumFields() != 1 {
		return false
	}
	se, ok := decl.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := se.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return selector.Sel.String() == requestCtx
}

var goPath string

func getPkg(file string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		log.Fatal(err)
	}
	pkg := strings.TrimPrefix(abs, goPath)
	pkg = strings.TrimPrefix(pkg, string(os.PathSeparator))
	return filepath.Dir(pkg)
}

type route struct {
	Pkg     string
	PkgPath string
	Method  string
	Func    string
	Path    string
}

func main() {
	var err error
	goPath = os.Getenv("GOPATH") + string(os.PathSeparator) + "src" + string(os.PathSeparator)
	curPkgPath := getPkg("." + string(os.PathSeparator) + "1")
	_, curPkg := filepath.Split(curPkgPath)
	fList := fileList(".")
	var routes []route
	for _, file := range fList {
		fset := token.NewFileSet()
		astTree, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			log.Fatal(err)
		}
		for _, decl := range astTree.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok {
				if filterFunc(f) {
					var rout route
					rout.Pkg = astTree.Name.Name
					rout.Func = f.Name.Name
					rout.PkgPath = getPkg(file)
					for _, comment := range f.Doc.List {
						if strings.HasPrefix(comment.Text, prefixPath) {
							rout.Path = strings.TrimSpace(comment.Text[len(prefixPath):])
						}
						if strings.HasPrefix(comment.Text, prefixMethod) {
							rout.Method = strings.TrimSpace(comment.Text[len(prefixMethod):])
						}
					}
					routes = append(routes, rout)
				}
			}
		}
	}
	imports := make(map[string]bool)
	for _, rout := range routes {
		if rout.PkgPath != curPkgPath {
			if _, ok := imports[rout.PkgPath]; !ok {
				imports[rout.PkgPath] = true
			}
		}
	}
	gen, err := os.Create("router_genrouter.go")
	if err != nil {
		log.Fatal(err)
	}
	defer gen.Close()
	fmt.Fprintln(gen, "//This file is generated by genrouter. DO NOT EDIT")
	fmt.Fprintf(gen, "package %v\n\n", curPkg)
	fmt.Fprintf(gen, "import (\n")
	fmt.Fprintf(gen, "\t\"github.com/buaazp/fasthttprouter\"\n")
	for pkg := range imports {
		fmt.Fprintf(gen, "\t\"%v\"\n", pkg)
	}
	fmt.Fprintf(gen, ")\n\n")
	fmt.Fprint(gen, "func Router() *fasthttprouter.Router {\n")
	fmt.Fprintf(gen, "\trouter := fasthttprouter.New()\n")
	for _, rout := range routes {
		meth := strings.ToUpper(rout.Method)
		switch meth {
		case "GET", "POST", "DELETE", "PUT", "OPTIONS", "HEAD":
			fmt.Fprintf(gen, "\trouter.%v(\"%v\", %v.%v)\n", meth, rout.Path, rout.Pkg, rout.Func)
		}
	}
	fmt.Fprintf(gen, "\treturn router\n")
	fmt.Fprintf(gen, "\n}\n")
}
