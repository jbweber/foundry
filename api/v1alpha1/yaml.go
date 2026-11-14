package v1alpha1

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// EncodeYAML writes vm as YAML that reads back as vm. It is yaml.Marshal,
// except where go-yaml writes YAML it cannot read itself: a string of
// several lines whose first line starts with a tab comes out as a literal
// block (|-) that go-yaml then refuses ("found a tab character where an
// indentation space is expected"). A spec holding one, in user data or an
// input file, would be stored in the domain's metadata, or printed, as YAML
// foundry cannot load.
//
// When the YAML does not parse, every string of several lines is written
// double-quoted instead, which escapes its tabs and line breaks. go-yaml
// picks the style only from a node tree, so the spec is encoded with such
// strings swapped for placeholders, and the strings put back into the tree.
func EncodeYAML(vm *VirtualMachine) ([]byte, error) {
	out, err := yaml.Marshal(vm)
	if err != nil {
		return nil, err
	}
	var check yaml.Node
	if yaml.Unmarshal(out, &check) == nil {
		return out, nil
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
	out, err = yaml.Marshal(&n)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(out, &check); err != nil {
		return nil, fmt.Errorf("the spec does not make YAML that reads back: %w", err)
	}
	return out, nil
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
