package bine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/google/renameio/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"github.com/tailscale/hujson"
)

type configAddition struct {
	path          string
	before, after []byte
	mode          os.FileMode
}

func (c *config) prepareAddition(tool *bin) (*configAddition, error) {
	info, err := os.Stat(c.path)
	if err != nil {
		return nil, err
	}
	before, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}
	cfg, err := unmarshalConfig(c.format, bytes.Clone(before))
	if err != nil {
		return nil, err
	}
	if cfg.Project != c.Project {
		return nil, errors.New("project configuration changed; reload it before adding a tool")
	}
	for _, existing := range cfg.Bins {
		if existing == nil {
			return nil, errors.New("null binary entry")
		}
		if existing.Name == tool.Name {
			return nil, fmt.Errorf("binary %q already exists", tool.Name)
		}
	}
	var after []byte
	switch c.format {
	case configFormatJSON:
		after, err = appendJSONBin(before, tool)
	case configFormatTOML:
		after, err = appendTOMLBin(before, tool, len(cfg.Bins))
	default:
		err = fmt.Errorf("unsupported config format %q", c.format)
	}
	if err != nil {
		return nil, err
	}
	updated, err := unmarshalConfig(c.format, bytes.Clone(after))
	if err != nil {
		return nil, fmt.Errorf("invalid configuration after adding tool: %w", err)
	}
	storedTool := *tool
	storedTool.source = nil
	if !reflect.DeepEqual(updated.Bins, append(cfg.Bins, &storedTool)) {
		return nil, errors.New("cannot add a tool without changing existing entries; check for conflicting bins fields")
	}
	return &configAddition{c.path, before, after, info.Mode().Perm()}, nil
}

func (e *configAddition) save() error {
	current, err := os.ReadFile(e.path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, e.before) {
		return errors.New("configuration changed while adding the tool; retry the command")
	}
	return renameio.WriteFile(e.path, e.after, e.mode, renameio.WithStaticPermissions(e.mode))
}

func appendJSONBin(data []byte, tool *bin) ([]byte, error) {
	tree, err := hujson.Parse(data)
	if err != nil {
		return nil, err
	}
	object, ok := tree.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New("configuration must be an object")
	}
	var bins *hujson.Value
	// Match the decoder's case folding and use the last matching field.
	for i := range object.Members {
		member := &object.Members[i]
		if strings.EqualFold(member.Name.Value.(hujson.Literal).String(), "bins") {
			bins = &member.Value
		}
	}
	if bins == nil {
		if err := tree.Patch([]byte(`[{"op":"add","path":"/bins","value":[]}]`)); err != nil {
			return nil, err
		}
		bins = tree.Find("/bins")
	}
	if bins.Value.Kind() == 'n' {
		bins.Value = &hujson.Array{}
	}
	tree.UpdateOffsets()
	data = tree.Pack()
	array, ok := bins.Value.(*hujson.Array)
	if !ok {
		return nil, errors.New("bins must be an array")
	}
	// Format only the new entry; existing values and comments remain untouched.
	multiline := bytes.Contains(data[bins.StartOffset:bins.EndOffset], []byte("\n"))
	if len(array.Elements) == 0 {
		multiline = bytes.Contains(data, []byte("\n"))
	}
	indent := jsonLineIndent(data, bins.StartOffset) + "  "
	if len(array.Elements) > 0 && multiline {
		indent = jsonLineIndent(data, array.Elements[0].StartOffset)
	}
	var entry []byte
	if multiline {
		entry, err = json.MarshalIndent(tool, indent, "  ")
	} else {
		entry, err = json.Marshal(tool)
	}
	if err != nil {
		return nil, err
	}
	value, err := hujson.Parse(entry)
	if err != nil {
		return nil, err
	}
	if multiline {
		value.BeforeExtra = []byte("\n" + indent)
		if len(array.Elements) == 0 && len(array.AfterExtra) == 0 {
			array.AfterExtra = []byte("\n" + jsonLineIndent(data, bins.StartOffset))
		}
	} else if len(array.Elements) > 0 {
		value.BeforeExtra = []byte(" ")
	}
	if len(array.Elements) > 0 && array.Elements[len(array.Elements)-1].AfterExtra != nil {
		value.AfterExtra = []byte{} // Preserve the array's trailing-comma style.
	}
	array.Elements = append(array.Elements, value)
	return tree.Pack(), nil
}

func jsonLineIndent(data []byte, offset int) string {
	start := bytes.LastIndexByte(data[:offset], '\n') + 1
	end := start
	for end < offset && (data[end] == ' ' || data[end] == '\t') {
		end++
	}
	return string(data[start:end])
}

func appendTOMLBin(data []byte, tool *bin, count int) ([]byte, error) {
	parser := unstable.Parser{}
	parser.Reset(data)
	root := true
	name := "bins"
	start, arrayStart := -1, -1
	for parser.NextExpression() {
		expr := parser.Expression()
		if expr.Kind == unstable.Table || expr.Kind == unstable.ArrayTable {
			root = false
		}
		if expr.Kind != unstable.ArrayTable && (!root || expr.Kind != unstable.KeyValue) {
			continue
		}
		keys := expr.Key()
		keys.Next()
		key := keys.Node()
		if strings.ToLower(string(key.Data)) != "bins" || keys.Next() {
			continue
		}
		// Keep the last matching spelling so a new table does not reset bins.
		name = string(key.Data)
		if expr.Kind == unstable.ArrayTable {
			start, arrayStart = -1, -1
			continue
		}
		start = int(key.Raw.Offset)
		keyEnd := start + int(key.Raw.Length)
		arrayStart = keyEnd + bytes.IndexByte(data[keyEnd:], '[')
	}
	if err := parser.Error(); err != nil {
		return nil, err
	}
	if arrayStart >= 0 {
		end, trailingComma := tomlArrayEnd(data, arrayStart)
		if count == 0 {
			// Empty arrays become tables so later upgrades can edit version keys.
			prefix := append(bytes.Clone(data[:start]), data[arrayStart+1:end-1]...)
			data = append(prefix, data[end:]...)
		} else {
			var buf bytes.Buffer
			entry := struct {
				Bin *bin `toml:"bin"`
			}{tool}
			if err := toml.NewEncoder(&buf).SetTablesInline(true).Encode(entry); err != nil {
				return nil, err
			}
			inline := strings.TrimSpace(strings.TrimPrefix(buf.String(), "bin = "))
			separator := " "
			if !trailingComma {
				separator = ", "
			}
			return append(append(bytes.Clone(data[:end-1]), []byte(separator+inline)...), data[end-1:]...), nil
		}
	}
	encoded, err := toml.Marshal(map[string][]*bin{name: {tool}})
	if err != nil {
		return nil, err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return append(append(data, '\n'), encoded...), nil
}

// The input has already passed TOML decoding. Find the outer array's closing
// bracket without mistaking brackets or commas inside strings and comments.
func tomlArrayEnd(data []byte, start int) (end int, trailingComma bool) {
	depth, last := 0, byte(0)
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '#':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case '"', '\'':
			quote := data[i]
			width := 1
			if i+2 < len(data) && data[i+1] == quote && data[i+2] == quote {
				width = 3
			}
			closing := bytes.Repeat([]byte{quote}, width)
			i += width
			for i < len(data) {
				if quote == '"' && data[i] == '\\' {
					i += 2
					continue
				}
				if bytes.HasPrefix(data[i:], closing) {
					i += width - 1
					if width == 3 {
						for i+1 < len(data) && data[i+1] == quote {
							i++
						}
					}
					break
				}
				i++
			}
			last = quote
		case '[':
			depth++
			last = '['
		case ']':
			depth--
			if depth == 0 {
				return i + 1, last == ','
			}
			last = ']'
		case ' ', '\t', '\r', '\n':
		default:
			last = data[i]
		}
	}
	return len(data), false
}
