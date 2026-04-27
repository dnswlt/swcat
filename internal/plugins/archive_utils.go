package plugins

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// readZipFile reads and returns the contents of the named file from the archive.
func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("file %q not found in archive", name)
}

// readAllProperties reads every .properties file in the archive and merges
// them into a single key/value map. On key collisions, last write wins.
func readAllProperties(zr *zip.Reader) (map[string]string, error) {
	props := map[string]string{}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".properties") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		for k, v := range parseProperties(data) {
			props[k] = v
		}
	}
	return props, nil
}

// parseProperties parses Java-style .properties content into a key/value map.
// Recognises '=' or ':' as separators; lines starting with '#' or '!' are comments.
func parseProperties(data []byte) map[string]string {
	props := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			continue
		}
		k := strings.TrimSpace(line[:sep])
		v := strings.TrimSpace(line[sep+1:])
		props[k] = v
	}
	return props
}

var unresolvedPropertyPlaceholderRE = regexp.MustCompile(`@@[^@]+@@`)

// replacePropertyPlaceholders replaces all @@key@@ placeholders in data with
// the corresponding value from props. Any placeholders that remain unresolved
// are replaced with a sentinel token so the output stays valid YAML.
func replacePropertyPlaceholders(data []byte, props map[string]string) []byte {
	if len(props) > 0 {
		pairs := make([]string, 0, len(props)*2)
		for k, v := range props {
			pairs = append(pairs, "@@"+k+"@@", v)
		}
		data = []byte(strings.NewReplacer(pairs...).Replace(string(data)))
	}
	return unresolvedPropertyPlaceholderRE.ReplaceAll(data, []byte("MISSING"))
}
