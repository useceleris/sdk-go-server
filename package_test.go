package celerisserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The public surface, written down once.
var exportedSurface = []string{
	"field Claims.AllowEcho",
	"field Claims.Channels",
	"field Claims.Permissions",
	"field Claims.Reference",
	"field Claims.Replay",
	"field SegmentClaim.Read",
	"field SegmentClaim.SegmentID",
	"field SegmentClaim.Write",
	"field SignerOptions.ClientID",
	"field SignerOptions.Clock",
	"field SignerOptions.SigningSecret",
	"func AllChannels",
	"func AllSegments",
	"func NewCredentialProvider",
	"func NewSigner",
	"func ReplayBacklog",
	"func ReplayLookback",
	"func RestrictedChannels",
	"func RestrictedSegments",
	"method Signer.Sign",
	"method SignerOptions.Format",
	"method SignerOptions.LogValue",
	"method SignerOptions.String",
	"type ChannelScope",
	"type Claims",
	"type ClaimsFunc",
	"type Replay",
	"type SegmentClaim",
	"type SegmentPermissions",
	"type Signer",
	"type SignerOptions",
}

func packageFiles(t *testing.T) map[string]*ast.File {
	t.Helper()

	paths, err := filepath.Glob("*.go")

	if err != nil {
		t.Fatal(err)
	}

	files := map[string]*ast.File{}
	fileSet := token.NewFileSet()

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fileSet, path, nil, 0)

		if err != nil {
			t.Fatal(err)
		}

		files[path] = file
	}

	return files
}

func TestExportedSurfaceIsPinned(t *testing.T) {
	var surface []string

	for _, file := range packageFiles(t) {
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if !declaration.Name.IsExported() {
					continue
				}

				if declaration.Recv == nil {
					surface = append(surface, "func "+declaration.Name.Name)

					continue
				}

				receiver := declaration.Recv.List[0].Type

				if star, ok := receiver.(*ast.StarExpr); ok {
					receiver = star.X
				}

				surface = append(surface, "method "+receiver.(*ast.Ident).Name+"."+declaration.Name.Name)
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					typeSpecification, ok := specification.(*ast.TypeSpec)

					if !ok || !typeSpecification.Name.IsExported() {
						continue
					}

					surface = append(surface, "type "+typeSpecification.Name.Name)

					if structure, ok := typeSpecification.Type.(*ast.StructType); ok {
						for _, field := range structure.Fields.List {
							for _, name := range field.Names {
								if name.IsExported() {
									surface = append(surface, "field "+typeSpecification.Name.Name+"."+name.Name)
								}
							}
						}
					}
				}
			}
		}
	}

	slices.Sort(surface)

	if !slices.Equal(surface, exportedSurface) {
		t.Fatalf("exported surface changed:\n%s", strings.Join(surface, "\n"))
	}
}

// The signing secret never leaves a trusted server: the package has no way to
// print or log, and starts no work when imported.
func TestPackageNeitherPrintsNorStartsWorkAtImport(t *testing.T) {
	for path, file := range packageFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.FuncDecl:
				if node.Recv == nil && node.Name.Name == "init" {
					t.Errorf("%s declares init", path)
				}
			case *ast.SelectorExpr:
				if packageName, ok := node.X.(*ast.Ident); ok {
					call := packageName.Name + "." + node.Sel.Name

					if packageName.Name == "log" || strings.HasPrefix(call, "fmt.Print") || strings.HasPrefix(call, "fmt.Fprint") || strings.HasPrefix(call, "fmt.Sprint") || call == "os.Stdout" || call == "os.Stderr" || packageName.Name == "slog" && node.Sel.Name != "Value" && node.Sel.Name != "StringValue" {
						t.Errorf("%s uses %s", path, call)
					}
				}
			case *ast.Ident:
				if node.Name == "println" || node.Name == "print" {
					t.Errorf("%s uses %s", path, node.Name)
				}
			}

			return true
		})
	}
}
