# gobake

`gobake` is a command-line program for embedding the content of files within a
Go program.

## Installation

1. [Install Go](https://golang.org/doc/install)
2. [Install Git](https://git-scm.com/downloads)
3. Using a shell with Git (such as Git Bash), run the following command:

```
go get -u github.com/anaminus/gobake
```

If you configured Go correctly, this will install gobake to `$GOPATH/bin`,
which will allow you run it directly from a shell.

## Usage

```bash
gobake [options] [file ...]
```

Option          | Description
----------------|------------
`-compress`     | Compress the content of the file using gzip.
`-export`       | Export generated functions.
`-output NAME`  | The name of the file to generate (defaults to "_gobake.go").
`-package NAME` | The name of the package (defaults to "main").

For each file given, the generated source file will contain a function with
the following signature:

```go
func() io.ReadCloser
```

The name of a function is similar to the name of the corresponding file as
passed to gobake. Non-letters are stripped off the front of the filename, and
any other non-letters are converted to `_`. For example, `.config/cfg.XML`
becomes `config_cfg_XML()`. The first letter will be uppercase if the
`-export` flag is given, and lowercase otherwise.

Each function returns a `io.ReadCloser`. This can be read to get the content
of the file, and should be closed afterwards.

If the `-compress` flag is given, then the content of the file will be
compressed with gzip. This only affects the size of the generated source file
and the compiled binary. It does not affect how the ReadCloser should be
handled.
