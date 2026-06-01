package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// manifestTemplates holds starter manifests for the declarative kinds that
// `yggdrasil new <kind>` can emit (distinct from the repo-scaffold kinds
// integration|surface, which clone a template repo). These are deliberately
// minimal, illustrative starting points — edit, then `yggdrasil apply -f`.
// The authoritative schema for each kind lives in yggdrasil-core/manifest/.
var manifestTemplates = map[string]string{
	"workflow": `apiVersion: yggdrasil.io/v1alpha1
kind: workflow
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  trigger:
    mode: manual
  steps:
    - id: first-step
      use:
        kind: integration
        family: kubernetes
        operation: observe_objects
      with:
        objects:
          - apiVersion: v1
            kind: Namespace
            metadata: { name: default }
`,
	"policy": `apiVersion: yggdrasil.io/v1alpha1
kind: policy
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  effect: allow            # allow | deny  (deny precedence)
  subjects: ["role:ops"]
  actions: ["manifest.write"]
  resources: ["workflow/*"]
  # condition: optional runtime predicate evaluated before each write
`,
	"role": `apiVersion: yggdrasil.io/v1alpha1
kind: role
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  description: "What this role is allowed to do"
  rules:
    - actions: ["manifest.read"]
      resources: ["*"]
`,
	"product": `apiVersion: yggdrasil.io/v1alpha1
kind: product
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  version: "0.1.0"
  renderer: kustomize
  target: {}
  reconcile:
    mode: manual
`,
	"auth_provider": `apiVersion: yggdrasil.io/v1alpha1
kind: auth_provider
metadata:
  name: {{name}}
  namespace: {{namespace}}
spec:
  type: oidc
  issuer: "https://accounts.example.com"
  client_id: "REPLACE_ME"
  client_secret_ref: "secret://yggdrasil/{{name}}#client_secret"
  scopes: ["openid", "email", "profile"]
`,
}

func isManifestTemplateKind(kind string) bool {
	_, ok := manifestTemplates[kind]
	return ok
}

func manifestTemplateKinds() []string {
	kinds := make([]string, 0, len(manifestTemplates))
	for k := range manifestTemplates {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// renderManifestTemplate substitutes {{name}}/{{namespace}} into the
// starter manifest for the kind.
func renderManifestTemplate(kind, name, namespace string) (string, error) {
	tmpl, ok := manifestTemplates[kind]
	if !ok {
		return "", fmt.Errorf("no manifest template for kind %q (have: %s)", kind, strings.Join(manifestTemplateKinds(), ", "))
	}
	return strings.NewReplacer("{{name}}", name, "{{namespace}}", namespace).Replace(tmpl), nil
}

// runNewManifest emits a starter manifest to stdout (or -o <file>).
func runNewManifest(kind string, args []string) error {
	fs := flag.NewFlagSet("new "+kind, flag.ContinueOnError)
	namespace := fs.String("namespace", "default", "manifest namespace")
	fs.StringVar(namespace, "n", "default", "short for --namespace")
	outFile := fs.String("o", "", "write to a file instead of stdout")
	name, _, flagArgs := splitGetPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if name == "" {
		name = "example"
	}
	rendered, err := renderManifestTemplate(kind, name, *namespace)
	if err != nil {
		return err
	}
	if *outFile != "" {
		if err := os.WriteFile(*outFile, []byte(rendered), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s starter to %s\n", kind, *outFile)
		fmt.Printf("edit it, then: yggdrasil apply -f %s\n", *outFile)
		return nil
	}
	fmt.Print(rendered)
	return nil
}
