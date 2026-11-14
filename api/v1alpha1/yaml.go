package v1alpha1

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// EncodeYAML writes vm as YAML that reads back as vm. It is yaml.Marshal,
// except where go-yaml writes a string of several lines in a form it does
// not read back as written:
//
//   - A first line that starts with a tab comes out as a literal block (|-)
//     that go-yaml then refuses ("found a tab character where an indentation
//     space is expected").
//   - A first line that is empty comes out as a block with an indentation
//     indicator (|4-); in a mapping that is an item of a sequence, such as an
//     input file or a fw_cfg entry, go-yaml reads the indicator from a
//     different column than it wrote it from, and the string silently reads
//     back as another one ("\n#" reads back as "", its "#" taken for a
//     comment).
//
// A spec holding such a string, in user data, an input file or a fw_cfg
// string, would be stored in the domain's metadata, or printed, as YAML
// foundry cannot load, or loads as a different spec.
//
// So the YAML is read back and written again; if that does not give the
// same YAML, every string of several lines is written double-quoted instead,
// which escapes its tabs and line breaks. go-yaml picks the style only from a
// node tree, so the spec is encoded with such strings swapped for
// placeholders, and the strings put back into the tree.
func EncodeYAML(vm *VirtualMachine) ([]byte, error) {
	want, err := yaml.Marshal(vm)
	if err != nil {
		return nil, err
	}
	if vm == nil || readsBackAs(want, want) {
		return want, nil
	}

	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	prefix := "foundry-yaml-" + hex.EncodeToString(token) + "-"
	held := map[string]string{}
	swapped := substituteStrings(reflect.ValueOf(vm), func(s string) string {
		if !strings.Contains(s, "\n") {
			return s
		}
		key := fmt.Sprintf("%s%d", prefix, len(held))
		held[key] = s
		return key
	})

	var n yaml.Node
	if err := n.Encode(swapped.Interface()); err != nil {
		return nil, err
	}
	restoreStrings(&n, held)
	out, err := yaml.Marshal(&n)
	if err != nil {
		return nil, err
	}
	if !readsBackAs(out, want) {
		return nil, fmt.Errorf("the spec does not make YAML that reads back")
	}
	return out, nil
}

// readsBackAs reports whether out reads back as a spec that yaml.Marshal
// writes as want. Comparing the YAML, not the specs, lets through what YAML
// does not keep apart and foundry does not either: a nil list and an empty
// one, a time beyond the second.
func readsBackAs(out, want []byte) bool {
	var got VirtualMachine
	if yaml.Unmarshal(out, &got) != nil {
		return false
	}
	again, err := yaml.Marshal(&got)
	return err == nil && bytes.Equal(again, want)
}

// substituteStrings returns a deep copy of v with every string s, map keys
// included, replaced by swap(s). v itself is left as it is.
func substituteStrings(v reflect.Value, swap func(string) string) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		r := reflect.New(v.Type()).Elem()
		r.SetString(swap(v.String()))
		return r
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		r := reflect.New(v.Type().Elem())
		r.Elem().Set(substituteStrings(v.Elem(), swap))
		return r
	case reflect.Struct:
		r := reflect.New(v.Type()).Elem()
		r.Set(v) // unexported fields too; the exported ones are replaced below
		for i := range v.NumField() {
			if r.Field(i).CanSet() {
				r.Field(i).Set(substituteStrings(v.Field(i), swap))
			}
		}
		return r
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		r := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			r.Index(i).Set(substituteStrings(v.Index(i), swap))
		}
		return r
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		r := reflect.MakeMapWithSize(v.Type(), v.Len())
		for it := v.MapRange(); it.Next(); {
			r.SetMapIndex(substituteStrings(it.Key(), swap), substituteStrings(it.Value(), swap))
		}
		return r
	}
	return v
}

// restoreStrings puts the held strings back in place of their placeholders,
// double-quoted. One that is not UTF-8 loses its !!str tag, so the encoder
// writes it as !!binary, as yaml.Marshal does, which reads back as the same
// string.
func restoreStrings(n *yaml.Node, held map[string]string) {
	if n.Kind == yaml.ScalarNode {
		if s, ok := held[n.Value]; ok {
			n.Value = s
			n.Style = yaml.DoubleQuotedStyle
			if !utf8.ValidString(s) {
				n.Tag = ""
				n.Style = 0
			}
		}
	}
	for _, c := range n.Content {
		restoreStrings(c, held)
	}
}
