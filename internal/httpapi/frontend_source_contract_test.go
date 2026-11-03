package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func verifyFrontendFeatureAssets(t *testing.T, client *http.Client, origin string, raw []byte) {
	t.Helper()
	source := string(raw)
	start := strings.Index(source, "// Frontend composition imports: begin")
	end := strings.Index(source, "// Frontend composition imports: end")
	if start < 0 || end <= start {
		t.Fatal("frontend feature composition is missing")
	}
	imports := regexp.MustCompile(`import \{ (\w+) \} from "\./([\w-]+\.js)";`).FindAllStringSubmatch(source[start:end], -1)
	if len(imports) == 0 {
		t.Fatal("frontend feature imports are empty")
	}
	for _, entry := range imports {
		response, err := client.Get(origin + "/" + entry[2])
		if err != nil {
			t.Fatal(err)
		}
		payload, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "javascript") || !strings.Contains(string(payload), "export function "+entry[1]+"(") || !strings.Contains(source, " = "+entry[1]+"({") {
			t.Fatalf("frontend feature is not served/wired: %s", entry[2])
		}
	}
}

// Source contracts inspect declarations from the real embedded feature files in
// their original order. This view is test-only; browser tests execute the module
// graph and the HTTP contract separately verifies the served module assets.
func readFrontendContractAsset(name string) ([]byte, error) {
	raw, err := embeddedAssets.ReadFile(name)
	if err != nil || name != "web/app.js" {
		return raw, err
	}
	return frontendContractSource(raw)
}

func frontendContractSource(raw []byte) ([]byte, error) {
	source := strings.ReplaceAll(string(raw), "\r\n", "\n")
	getterPattern := regexp.MustCompile(`\b(get[A-Z]\w*): \(\) => (\w+),`)
	getters := getterPattern.FindAllStringSubmatch(source, -1)
	for _, kind := range []string{"imports", "bindings"} {
		source = regexp.MustCompile(`(?s)// Frontend composition `+kind+`: begin.*?// Frontend composition `+kind+`: end\n`).ReplaceAllString(source, "")
	}
	marker := regexp.MustCompile(`// Frontend feature: ([\w-]+\.js)#(\w+)`)
	heads := regexp.MustCompile(`(?m)^  (?:async )?function (\w+)\(`)
	featureImports := ""
	seenModules := map[string]bool{}
	var failure error
	source = marker.ReplaceAllStringFunc(source, func(match string) string {
		parts := marker.FindStringSubmatch(match)
		payload, err := embeddedAssets.ReadFile("web/" + parts[1])
		if err != nil {
			failure = err
			return match
		}
		module := strings.ReplaceAll(string(payload), "\r\n", "\n")
		if !seenModules[parts[1]] {
			seenModules[parts[1]] = true
			if boundary := strings.Index(module, "export function "); boundary >= 0 {
				header := module[:boundary]
				featureImports += header
				for _, entry := range regexp.MustCompile(`import \{ ([^}]+) \} from ("[^"]+");`).FindAllStringSubmatch(header, -1) {
					for _, name := range strings.Split(entry[1], ",") {
						featureImports += "import { " + strings.TrimSpace(name) + " } from " + entry[2] + ";\n"
					}
				}
			}
		}
		locations := heads.FindAllStringSubmatchIndex(module, -1)
		declaration := ""
		for i, location := range locations {
			if module[location[2]:location[3]] != parts[2] {
				continue
			}
			end := strings.LastIndex(module, "\n  return {\n")
			if i+1 < len(locations) {
				end = locations[i+1][0]
			}
			if end <= location[0] {
				failure = fmt.Errorf("missing feature end: %s", match)
				return match
			}
			declaration = strings.TrimRight(module[location[0]:end], "\n\r ")
			break
		}
		if declaration == "" {
			variable := regexp.MustCompile(`(?m)^  ((?:const|let) ` + regexp.QuoteMeta(parts[2]) + ` = [^\n]+)`).FindStringSubmatch(module)
			if len(variable) < 2 {
				failure = fmt.Errorf("missing feature declaration: %s", match)
				return match
			}
			declaration = variable[1]
		}
		lines := strings.Split(declaration, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimPrefix(line, "  ")
		}
		declaration = strings.Join(lines, "\n")
		for _, getter := range getters {
			declaration = strings.ReplaceAll(declaration, getter[1]+"()", getter[2])
			setter := regexp.MustCompile(`set` + getter[1][3:] + `\((true|false)\)`)
			declaration = setter.ReplaceAllString(declaration, getter[2]+" = $1")
		}
		return declaration
	})
	if failure != nil {
		return nil, failure
	}
	return []byte(featureImports + source), nil
}
