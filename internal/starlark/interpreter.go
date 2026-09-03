// Package starlark provides the sandboxed Starlark interpreter used by catalog
// configuration. It deliberately exposes only catalog data and a small set of
// host functions; scripts have no filesystem, network, clock, or import access.
package starlark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	starlarkjson "go.starlark.net/lib/json"
	star "go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/encoding/protojson"
)

const maxExecutionSteps = 1_000_000

// Catalog is the part of a catalog repository available to a Starlark program.
// *repo.Repository satisfies this interface.
type Catalog interface {
	Entity(ref *catalog.Ref) catalog.Entity
	IAnnotation(entity catalog.Entity, key string) (string, bool)
}

// Link is the typed result produced by the link constructor exposed to scripts.
type Link struct {
	URL   string
	Title string
	Icon  string
	Type  string
	Group string
	Label string
}

// Program is a compiled Starlark link generator.
type Program struct {
	filename string
	program  *star.Program
}

var predeclaredNames = map[string]bool{
	"json":        true,
	"link":        true,
	"lookup_ref":  true,
	"iannotation": true,
}

// Compile parses and compiles a Starlark link generator. Imports are disabled:
// a generator must be a self-contained file defining links(entity).
func Compile(filename string, source []byte) (*Program, error) {
	_, program, err := star.SourceProgramOptions(
		&syntax.FileOptions{},
		filename,
		source,
		func(name string) bool { return predeclaredNames[name] },
	)
	if err != nil {
		return nil, err
	}
	if program.NumLoads() != 0 {
		module, pos := program.Load(0)
		return nil, fmt.Errorf("%s: load statements are not supported (attempted to load %q)", pos, module)
	}
	return &Program{filename: filename, program: program}, nil
}

// Filename returns the source filename used in diagnostics.
func (p *Program) Filename() string {
	return p.filename
}

type runtime struct {
	catalog Catalog
	entity  catalog.Entity
	values  map[string]star.Value
}

// Links executes links(entity) and returns its typed link results.
func (p *Program) Links(entity catalog.Entity, catalogStore Catalog) ([]Link, error) {
	if entity == nil {
		return nil, fmt.Errorf("cannot generate links for a nil entity")
	}
	if catalogStore == nil {
		return nil, fmt.Errorf("cannot generate links without a catalog")
	}

	rt := &runtime{
		catalog: catalogStore,
		entity:  entity,
		values:  make(map[string]star.Value),
	}
	entityValue, err := rt.entityValue(entity)
	if err != nil {
		return nil, fmt.Errorf("convert entity %s for Starlark: %w", entity.GetRef(), err)
	}

	predeclared := star.StringDict{
		"json":        starlarkjson.Module,
		"link":        star.NewBuiltin("link", newLink),
		"lookup_ref":  star.NewBuiltin("lookup_ref", rt.lookupRef),
		"iannotation": star.NewBuiltin("iannotation", rt.iannotation),
	}
	thread := &star.Thread{Name: "catalog links"}
	thread.SetMaxExecutionSteps(maxExecutionSteps)

	globals, err := p.program.Init(thread, predeclared)
	if err != nil {
		return nil, evaluationError(err)
	}
	globals.Freeze()

	linksValue, ok := globals["links"]
	if !ok {
		return nil, fmt.Errorf("%s: script must define links(entity)", p.filename)
	}
	linksFunc, ok := linksValue.(star.Callable)
	if !ok {
		return nil, fmt.Errorf("%s: links is %s, want function", p.filename, linksValue.Type())
	}

	result, err := star.Call(thread, linksFunc, star.Tuple{entityValue}, nil)
	if err != nil {
		return nil, evaluationError(err)
	}
	list, ok := result.(*star.List)
	if !ok {
		return nil, fmt.Errorf("%s: links(entity) returned %s, want list of link values", p.filename, result.Type())
	}

	links := make([]Link, 0, list.Len())
	for i := range list.Len() {
		value := list.Index(i)
		link, ok := value.(*linkValue)
		if !ok {
			return nil, fmt.Errorf("%s: links(entity) returned %s at index %d, want link", p.filename, value.Type(), i)
		}
		links = append(links, link.Link)
	}
	return links, nil
}

func evaluationError(err error) error {
	if evalErr, ok := err.(*star.EvalError); ok {
		return fmt.Errorf("%s", evalErr.Backtrace())
	}
	return err
}

type linkValue struct {
	Link
}

func (l *linkValue) String() string {
	return fmt.Sprintf("link(url=%q, title=%q)", l.URL, l.Title)
}
func (*linkValue) Type() string          { return "link" }
func (*linkValue) Freeze()               {}
func (*linkValue) Truth() star.Bool      { return star.True }
func (*linkValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: link") }

// newLink implements the typed Starlark constructor:
//
//	link(url, title="", icon="", type="", group="", label="")
func newLink(_ *star.Thread, fn *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var link Link
	if err := star.UnpackArgs(fn.Name(), args, kwargs,
		"url", &link.URL,
		"title?", &link.Title,
		"icon?", &link.Icon,
		"type?", &link.Type,
		"group?", &link.Group,
		"label?", &link.Label,
	); err != nil {
		return nil, err
	}
	if (link.Group == "") != (link.Label == "") {
		return nil, fmt.Errorf("link: group and label must either both be set or both be empty")
	}
	return &linkValue{Link: link}, nil
}

func (rt *runtime) lookupRef(_ *star.Thread, fn *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var refDict *star.Dict
	if err := star.UnpackArgs(fn.Name(), args, kwargs, "ref", &refDict); err != nil {
		return nil, err
	}
	ref, err := refFromDict(refDict)
	if err != nil {
		return nil, fmt.Errorf("lookup_ref: %w", err)
	}
	entity := rt.catalog.Entity(ref)
	if entity == nil {
		return star.None, nil
	}
	value, err := rt.entityValue(entity)
	if err != nil {
		return nil, fmt.Errorf("lookup_ref: convert %s: %w", ref, err)
	}
	return value, nil
}

func (rt *runtime) iannotation(_ *star.Thread, fn *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var key string
	if err := star.UnpackArgs(fn.Name(), args, kwargs, "key", &key); err != nil {
		return nil, err
	}
	value, ok := rt.catalog.IAnnotation(rt.entity, key)
	if !ok {
		return star.None, nil
	}
	return star.String(value), nil
}

func refFromDict(dict *star.Dict) (*catalog.Ref, error) {
	kind, err := requiredDictString(dict, "kind")
	if err != nil {
		return nil, err
	}
	name, err := requiredDictString(dict, "name")
	if err != nil {
		return nil, err
	}
	namespace, err := optionalDictString(dict, "namespace")
	if err != nil {
		return nil, err
	}
	return catalog.RefFromPB(&catalog_pb.Ref{Kind: kind, Namespace: namespace, Name: name})
}

func requiredDictString(dict *star.Dict, key string) (string, error) {
	value, found, err := dict.Get(star.String(key))
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("reference has no %q field", key)
	}
	stringValue, ok := star.AsString(value)
	if !ok {
		return "", fmt.Errorf("reference field %q is %s, want string", key, value.Type())
	}
	return stringValue, nil
}

func optionalDictString(dict *star.Dict, key string) (string, error) {
	value, found, err := dict.Get(star.String(key))
	if err != nil || !found {
		return "", err
	}
	stringValue, ok := star.AsString(value)
	if !ok {
		return "", fmt.Errorf("reference field %q is %s, want string", key, value.Type())
	}
	return stringValue, nil
}

func (rt *runtime) entityValue(entity catalog.Entity) (star.Value, error) {
	cacheKey := entity.GetRef().String()
	if value, ok := rt.values[cacheKey]; ok {
		return value, nil
	}

	pb := catalog.ToPB(entity)
	encoded, err := protojson.Marshal(pb)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	removeGeneratedLinks(object)

	value, err := toValue(object)
	if err != nil {
		return nil, err
	}
	value.Freeze()
	rt.values[cacheKey] = value
	return value, nil
}

// Link generation must not depend on repository map iteration order. A related
// entity may already have had generated links attached when lookup_ref resolves
// it, so expose only authored links to scripts.
func removeGeneratedLinks(entity map[string]any) {
	metadata, ok := entity["metadata"].(map[string]any)
	if !ok {
		return
	}
	links, ok := metadata["links"].([]any)
	if !ok {
		return
	}
	authored := links[:0]
	for _, item := range links {
		link, ok := item.(map[string]any)
		if ok {
			if generated, _ := link["isGenerated"].(bool); generated {
				continue
			}
		}
		authored = append(authored, item)
	}
	if len(authored) == 0 {
		delete(metadata, "links")
	} else {
		metadata["links"] = authored
	}
}

func toValue(value any) (star.Value, error) {
	switch value := value.(type) {
	case nil:
		return star.None, nil
	case bool:
		return star.Bool(value), nil
	case string:
		return star.String(value), nil
	case json.Number:
		return numberValue(value)
	case []any:
		items := make([]star.Value, len(value))
		for i, item := range value {
			converted, err := toValue(item)
			if err != nil {
				return nil, err
			}
			items[i] = converted
		}
		return star.NewList(items), nil
	case map[string]any:
		dict := star.NewDict(len(value))
		for key, item := range value {
			converted, err := toValue(item)
			if err != nil {
				return nil, err
			}
			if err := dict.SetKey(star.String(key), converted); err != nil {
				return nil, err
			}
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("cannot convert JSON value of type %T", value)
	}
}

func numberValue(number json.Number) (star.Value, error) {
	s := number.String()
	if strings.ContainsAny(s, ".eE") {
		value, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, err
		}
		return star.Float(value), nil
	}
	integer, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid integer %q", s)
	}
	return star.MakeBigInt(integer), nil
}
