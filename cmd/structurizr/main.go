package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/dnswlt/swcat/internal/structurizr"
)

const (
	StructurizrID = "structurizr.dsl.identifier"
)

func addElement(m map[string]structurizr.Element, entity structurizr.Element) {
	if _, found := m[entity.GetID()]; found {
		log.Fatalf("Duplicate ID: %s\n", entity.GetID())
	}
	m[entity.GetID()] = entity
}

func printAll(ws *structurizr.Workspace) {
	// Collect elements
	elements := map[string]structurizr.Element{}
	for _, ss := range ws.Model.SoftwareSystems {
		addElement(elements, ss)
		for _, cont := range ss.Containers {
			addElement(elements, cont)
			for _, comp := range cont.Components {
				addElement(elements, comp)
			}
		}
	}

	// Relationships
	kinds := map[string]int{}
	for _, ss := range ws.Model.SoftwareSystems {
		fmt.Printf("%s\n", ss.Name)
		for _, rel := range ss.Relationships {
			if rel.LinkedRelationshipID != "" {
				continue // Skip derived relationships
			}
			src := elements[rel.SourceID]
			dst := elements[rel.DestinationID]
			kinds[fmt.Sprintf("%s-%s", src.GetKind(), dst.GetKind())]++
			dir := "->"
			peer := dst.GetName()
			if src.GetID() != ss.ID {
				dir = "<-"
				peer = src.GetName()
			}
			fmt.Printf("  {%s} %s %s\n", rel.Description, dir, peer)
		}
		for _, cont := range ss.Containers {
			fmt.Printf("  %s\n", cont.Name)
			for _, rel := range cont.Relationships {
				if rel.LinkedRelationshipID != "" {
					continue // Skip derived relationships
				}
				src := elements[rel.SourceID]
				dst := elements[rel.DestinationID]
				kinds[fmt.Sprintf("%s-%s", src.GetKind(), dst.GetKind())]++
				dir := "->"
				peer := dst.GetName()
				if src.GetID() != cont.ID {
					dir = "<-"
					peer = src.GetName()
				}
				fmt.Printf("    {%s/%s} %s %s\n", rel.Description, rel.Technology, dir, peer)
			}
			for _, comp := range cont.Components {
				// Element
				suffix := ""
				if slices.Contains(comp.GetTags(), "API") {
					suffix += " (API)"
				}
				fmt.Printf("    %s%s\n", comp.Name, suffix)
				// Relationships
				for _, rel := range comp.Relationships {
					if rel.LinkedRelationshipID != "" {
						continue // Skip derived relationships
					}
					src := elements[rel.SourceID]
					dst := elements[rel.DestinationID]
					kinds[fmt.Sprintf("%s-%s", src.GetKind(), dst.GetKind())]++

					dir := "->"
					peer := dst.GetName()
					if src.GetID() != comp.ID {
						dir = "<-"
						peer = src.GetName()
					}
					fmt.Printf("      {%s} %s %s\n", rel.Description, dir, peer)
				}
			}
		}
	}
	fmt.Println()
	for kind, cnt := range kinds {
		fmt.Printf("%s: %d\n", kind, cnt)
	}

}

func main() {
	workspaceFlag := flag.String("workspace", "", "Workspace JSON file")
	flag.Parse()

	f, err := os.Open(*workspaceFlag)
	if err != nil {
		log.Fatalf("Could not open workspace file: %v", err)
	}
	var workspace structurizr.Workspace

	if err := json.NewDecoder(f).Decode(&workspace); err != nil {
		log.Fatalf("Could not read workspace: %v", err)
	}

	printAll(&workspace)
}
