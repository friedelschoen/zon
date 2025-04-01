package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"hash/crc64"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type RingBuffer struct {
	Content []byte
	Index   int
	Size    int
}

func (b *RingBuffer) Write(p []byte) (int, error) {
	for _, chr := range p {
		b.Content[b.Index] = chr
		b.Index = (b.Index + 1) % len(b.Content)
		b.Size = min(b.Size+1, len(b.Content))
	}
	return len(p), nil
}

func (b *RingBuffer) Get() []byte {
	result := make([]byte, b.Size)
	for i := range b.Size {
		result[i] = b.Content[(i+b.Index)%len(b.Content)]
	}
	return result
}

type Parser struct {
	dec *json.Decoder
	cwd string
}

type Object struct {
	defines  map[string]any
	includes []string
	extends  []string
	values   map[string]any
}

func (p *Parser) parseValue() (any, error) {
	token, err := p.dec.Token()
	if err != nil {
		return nil, err
	}

	switch tok := token.(type) {
	case json.Delim:
		switch tok {
		case '{':
			return p.parseMap()
		case '[':
			return p.parseArray()
		}
	case string:
		if strings.HasPrefix(tok, "./") {
			return path.Join(p.cwd, tok), nil
		}
		return tok, nil
	default:
		return tok, nil
	}
	return nil, fmt.Errorf("unexpected token: %v", token)
}

func (p *Parser) parseMap() (*Object, error) {
	result := &Object{
		defines: make(map[string]any),
		values:  make(map[string]any),
	}

	for p.dec.More() {
		key, err := p.dec.Token()
		if err != nil {
			return nil, err
		}
		keyStr, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key, got %T", key)
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		switch keyStr {
		case "@define":
			defs, ok := value.(*Object)
			if !ok {
				return nil, fmt.Errorf("@define must be a map, got %T", value)
			}
			maps.Copy(result.defines, defs.values)
		case "@expand":
			name, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("@expand variable must be string, got %T", value)
			}
			result.extends = append(result.extends, name)
		case "@include":
			file, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("@include must be a string, got %T", value)
			}
			result.includes = append(result.includes, file)
		default:
			result.values[keyStr] = value
		}
	}
	_, err := p.dec.Token() // Consume '}'
	return result, err
}

func (p *Parser) parseArray() ([]any, error) {
	var result []any
	for p.dec.More() {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	_, err := p.dec.Token() // Consume ']'
	return result, err
}

func parseFile(filename string) (any, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()
	cwd, _ := filepath.Abs(path.Dir(filename))
	parser := Parser{dec: json.NewDecoder(file), cwd: cwd}
	return parser.parseValue()
}

func interpolate(str string, scope map[string]any) (any, error) {
	if str == "" {
		return str, nil
	}
	if str[0] == '@' {
		varName := str[1:]
		replacement, found := scope[varName]
		if !found {
			return nil, fmt.Errorf("undefined variable: %s", varName)
		}
		return resolve(replacement, scope)
	}
	var builder strings.Builder
	for len(str) > 0 {
		startIdx := strings.Index(str, "{{")
		if startIdx == -1 {
			break
		}
		builder.WriteString(str[:startIdx])
		str = str[startIdx:]
		endIdx := strings.Index(str, "}}")
		if endIdx == -1 {
			return nil, fmt.Errorf("unmatched {{ in string: %s", str)
		}
		varName := str[2:endIdx]
		replacement, found := scope[varName]
		if !found {
			return nil, fmt.Errorf("undefined variable: %s", varName)
		}
		replacementStr, valid := replacement.(string)
		if !valid {
			return nil, fmt.Errorf("variable %s must be a string, got %T", varName, replacement)
		}
		builder.WriteString(replacementStr)
		str = str[endIdx+2:]
	}
	builder.WriteString(str)
	return builder.String(), nil
}

func resolve(ast any, scope map[string]any) (any, error) {
	switch ast := ast.(type) {
	case *Object:
		for len(ast.includes) > 0 || len(ast.extends) > 0 {
			var otherast any
			if len(ast.includes) > 0 {
				inclpath := ast.includes[0]
				ast.includes = ast.includes[1:]
				var err error
				otherast, err = parseFile(inclpath)
				if err != nil {
					return nil, err
				}
			} else if len(ast.extends) > 0 {
				extname := ast.extends[0]
				ast.extends = ast.extends[1:]
				var ok bool
				otherast, ok = scope[extname]
				if !ok {
					return nil, fmt.Errorf("not in scope: %s\n", extname)
				}
			}
			otherobject, ok := otherast.(*Object)
			if !ok {
				return nil, fmt.Errorf("@includes expects object")
			}
			maps.Copy(ast.defines, otherobject.defines)
			maps.Copy(ast.values, otherobject.values)
			ast.extends = append(ast.extends, otherobject.extends...)
			ast.includes = append(ast.extends, otherobject.includes...)
		}
		newscope := maps.Clone(scope)
		maps.Copy(newscope, ast.defines)
		var err error
		for k, v := range ast.values {
			ast.values[k], err = resolve(v, newscope)
			if err != nil {
				return nil, err
			}
		}
		if _, ok := ast.values["@output"]; ok {
			return output(ast.values)
		}
		if unwrap, ok := ast.values["@"]; ok {
			return unwrap, nil
		}
	case []any:
		var err error
		for i, elem := range ast {
			ast[i], err = resolve(elem, scope)
			if err != nil {
				return nil, err
			}
		}
	case string:
		return interpolate(ast, scope)
	}
	return ast, nil
}

func output(result map[string]any) (string, error) {
	hashlib := crc64.New(crc64.MakeTable(crc64.ECMA))
	enc := json.NewEncoder(hashlib)
	hashValue(hashlib, enc, result)
	hashstr := hex.EncodeToString(hashlib.Sum(nil))

	cwd, _ := os.Getwd()
	outdir := path.Join(cwd, "store", hashstr)
	if _, err := os.Stat(outdir); err == nil {
		return outdir, nil
	}
	os.RemoveAll(outdir)
	success := false
	defer func() {
		if !success {
			os.RemoveAll(outdir)
		}
	}()

	install := ""
	switch outputValue := result["@output"].(type) {
	case string:
		install = outputValue
	case []any:
		var builder strings.Builder
		for i, elem := range outputValue {
			if i > 0 {
				builder.WriteByte('\n')
			}
			elemStr, ok := elem.(string)
			if !ok {
				return "", fmt.Errorf("@output expected string or string[], got %T in array", elem)
			}
			builder.WriteString(elemStr)
		}
		install = builder.String()
	default:
		return "", fmt.Errorf("@output must be a string")
	}

	interpreter := "sh"
	if interpreterAny, ok := result["@interpreter"]; ok {
		if str, ok := interpreterAny.(string); ok {
			interpreter = str
		} else {
			return "", fmt.Errorf("@interpreter must be a string")
		}
	}

	builddir, err := os.MkdirTemp("", "bake-")
	if err != nil {
		return "", err
	}
	environ := append(os.Environ(), "out="+outdir)
	for key, value := range result {
		if key != "" && key[0] == '$' {
			enc, err := encodeEnviron(value, true)
			if err != nil {
				return "", err
			}
			environ = append(environ, key[1:]+"="+enc)
		}
	}

	os.MkdirAll("logs", 0755)
	logfile, err := os.Create(path.Join("logs", hashstr+".txt"))
	if err != nil {
		logfile = os.Stdout
	}
	logbuf := &RingBuffer{Content: make([]byte, 128)}
	stdout := io.MultiWriter(logfile, logbuf)

	if nameAny, ok := result["@name"]; ok {
		if name, ok := nameAny.(string); ok {
			fmt.Printf("building %s (%s)\n", hashstr, name)
		}
	} else {
		fmt.Printf("building %s\n", hashstr)
	}

	cmd := exec.Command(interpreter, "-e", "-c", install)
	cmd.Env = environ
	cmd.Dir = builddir
	cmd.Stdin = nil
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Run(); err != nil {
		fmt.Println(string(logbuf.Get()))
		return "", err
	}

	success = true
	return outdir, nil
}

func hashValue(hashlib hash.Hash, encoder *json.Encoder, value any) {
	switch value := value.(type) {
	case map[string]any:
		keys := slices.Collect(maps.Keys(value))
		slices.Sort(keys)
		for _, k := range keys {
			hashlib.Write([]byte(k))
			hashValue(hashlib, encoder, value[k])
		}
	default:
		encoder.Encode(value)
	}
}

func encodeEnviron(value any, root bool) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case bool:
		if value {
			return "1", nil
		}
		return "0", nil
	case []any:
		if !root {
			return "", fmt.Errorf("unable to encode nested %T", value)
		}
		var builder strings.Builder
		for i, elem := range value {
			if i > 0 {
				builder.WriteByte(' ')
			}
			enc, err := encodeEnviron(elem, false)
			if err != nil {
				return "", err
			}
			builder.WriteString(enc)
		}
		return builder.String(), nil
	case map[string]any:
		if !root {
			return "", fmt.Errorf("unable to encode nested %T", value)
		}
		var builder strings.Builder
		first := true
		for key, elem := range value {
			if !first {
				builder.WriteByte(' ')
			}
			first = false
			builder.WriteString(key)
			builder.WriteByte('=')
			enc, err := encodeEnviron(elem, false)
			if err != nil {
				return "", err
			}
			builder.WriteString(enc)
		}
		return builder.String(), nil
	default:
		return "", fmt.Errorf("unable to encode %T", value)
	}
}

func main() {
	os.MkdirAll("store", 0755)
	ast, err := parseFile("../../data.json")
	if err != nil {
		panic(err)
	}
	res, err := resolve(ast, map[string]any{})
	if err != nil {
		panic(err)
	}
	switch res := res.(type) {
	case string:
		fmt.Println(res)
	case []any:
		for _, r := range res {
			fmt.Println(r)
		}
	}
}
