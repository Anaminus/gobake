package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"github.com/anaminus/but"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Compressor interface {
	// Encode actual data.
	Encode([]byte) []byte
	// Return list of packages required by function body.
	Imports() []string
	// Receive variable name, return function body.
	FuncDecoder(string) string
}

type noCompressor struct{}

func (noCompressor) Encode(b []byte) []byte {
	return b
}

func (noCompressor) Imports() []string {
	return []string{"ioutil", "strings"}
}

func (noCompressor) FuncDecoder(v string) string {
	return "return ioutil.NopCloser(strings.NewReader(" + v + "))"
}

type gzipCompressor struct{}

func (gzipCompressor) Encode(b []byte) []byte {
	var buf strings.Builder
	w := gzip.NewWriter(&buf)
	_, err := w.Write(b)
	but.IfFatal(err, "write gzip")
	but.IfFatal(w.Close(), "close gzip")
	return []byte(buf.String())
}

func (gzipCompressor) Imports() []string {
	return []string{"gzip", "strings"}
}

func (gzipCompressor) FuncDecoder(v string) string {
	return "gr, _ := gzip.NewReader(strings.NewReader(" + v + "))\n\treturn gr"
}

type Declaration interface {
	// Return list of package required by declaration.
	Imports() []string
	// Return declaration.
	FormatDeclare(value []byte, name, typ string, compress Compressor) string
}

type constDecl struct{}

func (constDecl) Imports() []string { return nil }
func (constDecl) FormatDeclare(value []byte, name, typ string, compress Compressor) string {
	return "const " + name + " = " + formatValue(16, 1, typ, compress.Encode(value))
}

type varDecl struct{}

func (varDecl) Imports() []string { return nil }
func (varDecl) FormatDeclare(value []byte, name, typ string, compress Compressor) string {
	return "var " + name + " = " + formatValue(16, 1, typ, compress.Encode(value))
}

type funcDecl struct{}

func (funcDecl) Imports() []string { return []string{"io"} }
func (funcDecl) FormatDeclare(value []byte, name, typ string, compress Compressor) string {
	return `func ` + name + "() io.ReadCloser {\n\tconst a = " +
		formatValue(16, 2, typ, compress.Encode(value)) +
		"\t" + compress.FuncDecoder("a") + "\n}\n"
}

// Format data as string. Wrap specifices the number of bytes at which to wrap.
// Typ specifies an optional type that encloses the generated string.
func formatValue(wrap, indent int, typ string, b []byte) string {
	const hextable = "0123456789abcdef"
	if typ == "string" {
		typ = ""
	}
	if len(b) == 0 {
		if typ == "" {
			return "\"\"\n"
		}
		return typ + "(\"\")\n"
	}
	var s strings.Builder
	if typ != "" {
		s.WriteString(typ)
		s.WriteByte('(')
	}
	if wrap > 0 && len(b) > wrap {
		s.WriteString("\"\" +\n")
	} else {
		s.WriteByte('"')
	}
	for i := 0; i < len(b); i++ {
		if len(b) > wrap && wrap > 0 && i%wrap == 0 {
			for i := 0; i < indent; i++ {
				s.WriteString("\t")
			}
			s.WriteString("\"")
		}
		s.WriteString("\\x")
		s.WriteByte(hextable[b[i]>>4])
		s.WriteByte(hextable[b[i]&0x0f])
		if i == len(b)-1 {
			s.WriteByte('"')
			if typ != "" {
				s.WriteByte(')')
			}
			s.WriteByte('\n')
		} else if wrap > 0 && i%wrap == wrap-1 {
			s.WriteString("\" +\n")
		}
	}
	return s.String()
}

// Sanitize a string so that it's suitable as a variable name.
func getDeclName(name string, export bool) string {
	s := []rune{}
	for _, r := range name {
		switch {
		case unicode.IsLetter(r):
			if len(s) == 0 {
				if export {
					s = append(s, unicode.ToUpper(r))
				} else {
					s = append(s, unicode.ToLower(r))
				}
			} else {
				s = append(s, r)
			}
		default:
			if len(s) > 0 {
				s = append(s, '_')
			}
		}
	}
	return string(s)
}

func main() {
	var flags struct {
		Decl     string
		Compress string
		Export   bool
		Import   string
		Name     string
		Output   string
		Package  string
		Type     string
	}

	flag.StringVar(&flags.Decl, "decl", "func", `How to declare the value. Can be "func", "const", or "var".`)
	flag.StringVar(&flags.Compress, "compress", "", `How to compress the value. Can be "" (none), or "gzip".`)
	flag.BoolVar(&flags.Export, "export", false, `Whether the declaration should be exported.`)
	flag.StringVar(&flags.Import, "import", "", `An optional package to import. Usually combined with -type.`)
	flag.StringVar(&flags.Name, "name", "", `The name of the declared value. Defaults to the name of the input file.`)
	flag.StringVar(&flags.Output, "output", "", `The name of the generated file. Writes to stdout if empty.`)
	flag.StringVar(&flags.Package, "package", "", `The name of the package. Determined by output location if empty, or "main" if all else fails.`)
	flag.StringVar(&flags.Type, "type", "string", `The type of the declared value. Must be convertable to a string.`)
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gobake [options] [file]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Reads from stdin if file is omitted.")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()

	var imports []string

	var declaration Declaration
	switch flags.Decl {
	case "var":
		declaration = varDecl{}
	case "const":
		flags.Type = ""
		declaration = constDecl{}
	case "func":
		fallthrough
	default:
		flags.Type = ""
		declaration = funcDecl{}
	}
	imports = append(imports, declaration.Imports()...)

	var compressor Compressor
	switch flags.Compress {
	case "gzip":
		compressor = gzipCompressor{}
	default:
		compressor = noCompressor{}
	}
	if _, ok := declaration.(funcDecl); ok {
		imports = append(imports, compressor.Imports()...)
	}

	if flags.Import != "" {
		imports = append(imports, flags.Import)
	}
	sort.Strings(imports)

	if flags.Package == "" {
		if flags.Output == "" {
			flags.Package = "main"
		} else {
			pkg, err := build.ImportDir(filepath.Dir(flags.Output), 0)
			if err != nil || pkg.Name == "" {
				flags.Package = "main"
			} else {
				flags.Package = pkg.Name
			}
		}
	}

	var o strings.Builder
	o.WriteString("// File generated by \"gobake")
	for i := 1; i < len(os.Args); i++ {
		o.WriteByte(' ')
		o.WriteString(os.Args[i])
	}
	o.WriteString("\"\n// DO NOT EDIT!\n\npackage ")
	o.WriteString(flags.Package)
	o.WriteString("\n\n")
	if len(imports) > 0 {
		o.WriteString("import (\n")
		for _, imp := range imports {
			fmt.Fprintf(&o, "\t%#v\n", imp)
		}
		o.WriteString(")\n\n")
	}

	name := flags.Name
	b := []byte{}
	if flag.NArg() == 0 {
		var err error
		b, err = ioutil.ReadAll(os.Stdin)
		but.IfFatal(err, "read stdin")
		if name == "" {
			name = "stdin"
		}
	} else {
		var err error
		b, err = ioutil.ReadFile(flag.Arg(0))
		but.IfFatal(err, "read file")
		if name == "" {
			name = filepath.Base(flag.Arg(0))
			name = name[:len(name)-len(filepath.Ext(name))]
		}
	}

	name = getDeclName(name, flags.Export)
	o.WriteString(declaration.FormatDeclare(b, name, flags.Type, compressor))
	if flags.Output == "" {
		if flags.Package == "" {
			flags.Package = "main"
		}
		_, err := os.Stdout.Write([]byte(o.String()))
		but.IfFatal(err, "write stdout")
	} else {
		but.IfFatal(ioutil.WriteFile(flags.Output, []byte(o.String()), 0666), "write file")
	}
}
