package backstage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

func runDot(ctx context.Context, dotSource string) ([]byte, error) {
	// Command: dot -Tsvg
	cmd := exec.CommandContext(ctx, "dot", "-Tsvg")

	// Provide the DOT source on stdin and capture stdout/stderr
	// Use CombinedOutput to get useful error messages in case dot fails.
	cmd.Stdin = nil // we'll set via a pipe below
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, dotSource)
	}()

	output, err := cmd.CombinedOutput() // will wait until process exits
	if err != nil {
		// CombinedOutput returns output (stdout+stderr) even on error - include it for debugging.
		return output, fmt.Errorf("dot failed: %w; output: %s", err, output)
	}

	return output, nil
}

// GenerateComponentSVG generates an SVG for the given component.
func GenerateComponentSVG(r *Repository, name string) ([]byte, error) {
	component := r.Component(name)
	if component == nil {
		return nil, fmt.Errorf("component %s does not exist", name)
	}
	qn := component.GetQName()

	var sb strings.Builder
	sb.WriteString(`digraph {`)
	sb.WriteString(`rankdir="LR"`)
	sb.WriteString(`fontname="sans-serif"`)
	sb.WriteString(`splines="spline"`)
	sb.WriteString(`fontsize="11"`)
	sb.WriteString(`node[shape="box",fontname="sans-serif",fontsize="11",style="filled,rounded"]`)
	sb.WriteString(`edge[fontname="sans-serif",fontsize="11",minlen="4"]`)
	// Component
	fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="lightblue"]`, qn, qn, qn)

	// "Incoming" dependencies
	// - Owner
	// - System
	// - Provided APIs
	// - Other entities with a DependsOn relationship to this entity
	owner := r.Group(component.Spec.Owner)
	if owner != nil {
		ownerQn := owner.GetQName()
		fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="sandybrown",shape="ellipse"]`, ownerQn, ownerQn, ownerQn)
		fmt.Fprintf(&sb, `"%s" -> "%s"`, ownerQn, qn)
	}
	system := r.System(component.Spec.System)
	if system != nil {
		systemQn := system.GetQName()
		fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="lightsteelblue"]`, systemQn, systemQn, systemQn)
		fmt.Fprintf(&sb, `"%s" -> "%s"`, systemQn, qn)
	}
	for _, a := range component.Spec.ProvidesAPIs {
		api := r.API(a)
		apiQn := api.GetQName()
		fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="plum"]`, apiQn, apiQn, apiQn)
		fmt.Fprintf(&sb, `"%s" -> "%s"[dir="back",arrowtail="empty"]`, apiQn, qn)
	}
	for _, d := range component.Spec.dependents {
		e := r.Entity(d)
		switch x := e.(type) {
		case *Component:
			xQn := x.GetQName()
			fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="lightblue"]`, xQn, xQn, xQn)
			fmt.Fprintf(&sb, `"%s" -> "%s"`, xQn, qn)
		case *Resource:
			xQn := x.GetQName()
			fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="azure"]`, xQn, xQn, xQn)
			fmt.Fprintf(&sb, `"%s" -> "%s"`, xQn, qn)
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		api := r.API(a)
		apiQn := api.GetQName()
		fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="plum"]`, apiQn, apiQn, apiQn)
		fmt.Fprintf(&sb, `"%s" -> "%s"[dir="back",arrowtail="empty"]`, qn, apiQn)
	}
	for _, d := range component.Spec.DependsOn {
		e := r.Entity(d)
		switch x := e.(type) {
		case *Component:
			xQn := x.GetQName()
			fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="lightblue"]`, xQn, xQn, xQn)
			fmt.Fprintf(&sb, `"%s" -> "%s"`, qn, xQn)
		case *Resource:
			xQn := x.GetQName()
			fmt.Fprintf(&sb, `"%s"[id="%s",label="%s",fillcolor="azure"]`, xQn, xQn, xQn)
			fmt.Fprintf(&sb, `"%s" -> "%s"`, qn, xQn)
		}
	}

	sb.WriteString("}")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := runDot(ctx, sb.String())
	if err != nil {
		return nil, err
	}
	// Cut off <?xml ?> header and only return the <svg>
	if idx := bytes.Index(output, []byte("<svg")); idx > 0 {
		output = output[idx:]
	}

	return output, nil

}
