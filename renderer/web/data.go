package web

import (
	"fmt"
	"sort"
	"strings"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/hypercard"
	pdflayout "github.com/glthr/DeMystify/renderer/layout"
)

// Options selects optional local artwork and original card geometry. No
// external programs, network services, or script runtime are used by Write.
type Options struct {
	AssetsDir string
	InputDir  string // Protected original input directory, when known by the caller.
	Corpus    *hypercard.Corpus
	PDFLayout *pdflayout.Drawing // Optional Graphviz drawing, prepared by the caller.
}

// Report describes the completed, local HTML export.
type Report struct {
	Dir           string
	IndexPath     string
	Nodes         int
	Edges         int
	Images        int
	MissingImages int
	Warnings      []string
}

type graphData struct {
	Nodes     []webNode          `json:"nodes"`
	Edges     []webEdge          `json:"edges"`
	Meta      webMeta            `json:"meta"`
	PDFLayout *pdflayout.Drawing `json:"pdfLayout,omitempty"`
}

type webNode struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Stack        string   `json:"stack"`
	OriginalName string   `json:"originalName,omitempty"`
	AssetName    string   `json:"assetName,omitempty"`
	Image        string   `json:"image,omitempty"`
	Thumbnail    string   `json:"thumbnail,omitempty"`
	ImageWidth   int      `json:"imageWidth,omitempty"`
	ImageHeight  int      `json:"imageHeight,omitempty"`
	X            float64  `json:"x"`
	Y            float64  `json:"y"`
	Width        float64  `json:"width"`
	Height       float64  `json:"height"`
	Attributes   []string `json:"attributes"`
	Zones        []zone   `json:"zones,omitempty"`
	ViewWidth    int      `json:"viewWidth,omitempty"`
	ViewHeight   int      `json:"viewHeight,omitempty"`
}

type zone struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Layer   string `json:"layer"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Script  string `json:"script,omitempty"`
	Hidden  bool   `json:"hidden"`
	Enabled bool   `json:"enabled"`
}

type webEdge struct {
	Source         int64    `json:"source"`
	Target         int64    `json:"target"`
	Attributes     []string `json:"attributes"`
	TransitivityID int64    `json:"transitivityId"`
}

type bounds struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
}

type layoutGroup struct {
	Name string `json:"name"`
	bounds
}

type webMeta struct {
	Cards           uint          `json:"cards"`
	Stacks          uint          `json:"stacks"`
	Nodes           int           `json:"nodes"`
	Edges           int           `json:"edges"`
	Images          int           `json:"images"`
	MissingImages   int           `json:"missingImages"`
	Layout          bounds        `json:"layout"`
	LayoutAlgorithm string        `json:"layoutAlgorithm"`
	Groups          []layoutGroup `json:"groups"`
	Zones           int           `json:"zones"`
	DefaultLayout   string        `json:"defaultLayout"`
}

func collectData(metadata *common.Metadata, corpus *hypercard.Corpus) (graphData, error) {
	if metadata == nil {
		return graphData{}, fmt.Errorf("HTML export requires graph metadata")
	}
	data := graphData{Nodes: []webNode{}, Edges: []webEdge{}}
	seen := make(map[int64]bool, len(metadata.Nodes))
	const maxSafeID = int64(1<<53 - 1)
	for _, node := range metadata.Nodes {
		if node == nil {
			return graphData{}, fmt.Errorf("HTML export contains a nil node")
		}
		if node.GraphID < -maxSafeID || node.GraphID > maxSafeID || seen[node.GraphID] {
			return graphData{}, fmt.Errorf("HTML export contains duplicate or unsafe node ID %d", node.GraphID)
		}
		seen[node.GraphID] = true
		stack := node.StackName
		if stack == "" && node.IsOfType(common.IsStack) {
			stack = node.Name
		}
		item := webNode{ID: node.GraphID, Name: node.Name, Stack: stack, Width: 180, Height: 56,
			Attributes: nodeAttributes(node.Attributes)}
		if node.OriginalName != nil {
			item.OriginalName = *node.OriginalName
		}
		if node.SecondaryName != nil {
			item.AssetName = *node.SecondaryName
		}
		data.Nodes = append(data.Nodes, item)
	}
	sort.Slice(data.Nodes, func(i, j int) bool { return data.Nodes[i].ID < data.Nodes[j].ID })
	for _, edge := range metadata.Edges {
		if edge == nil || edge.Source == nil || edge.Target == nil {
			return graphData{}, fmt.Errorf("HTML export contains an edge without endpoints")
		}
		if !seen[edge.Source.GraphID] || !seen[edge.Target.GraphID] {
			return graphData{}, fmt.Errorf("HTML edge %d → %d refers to a missing node", edge.Source.GraphID, edge.Target.GraphID)
		}
		if edge.TransitivityID < -maxSafeID || edge.TransitivityID > maxSafeID {
			return graphData{}, fmt.Errorf("HTML edge has unsafe transitivity ID %d", edge.TransitivityID)
		}
		data.Edges = append(data.Edges, webEdge{Source: edge.Source.GraphID, Target: edge.Target.GraphID,
			Attributes: edgeAttributes(edge.Attributes), TransitivityID: edge.TransitivityID})
	}
	// Preserve every edge record, including equal parallel records and loops.
	// Sorting makes the output independent of caller map/iteration order.
	sort.SliceStable(data.Edges, func(i, j int) bool {
		a, b := data.Edges[i], data.Edges[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.TransitivityID != b.TransitivityID {
			return a.TransitivityID < b.TransitivityID
		}
		return strings.Join(a.Attributes, "\x00") < strings.Join(b.Attributes, "\x00")
	})
	data.Meta = webMeta{Cards: metadata.TotalCards, Stacks: metadata.TotalStacks,
		Nodes: len(data.Nodes), Edges: len(data.Edges), LayoutAlgorithm: "deterministic grouped force with rectangle packing", DefaultLayout: "atlas"}
	if corpus != nil {
		if err := addZones(&data, corpus); err != nil {
			return graphData{}, err
		}
	}
	return data, nil
}

func nodeAttributes(attributes []common.NodeAttribute) []string {
	result := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		name, ok := map[common.NodeAttribute]string{
			common.IsCard: "card", common.IsStack: "stack", common.IsVirtual: "virtual",
			common.ContainsBluePage: "containsBluePage", common.ContainsRedPage: "containsRedPage", common.ContainsWhitePage: "containsWhitePage",
			common.IsIsolated: "isolated", common.IsSource: "source", common.IsSink: "sink",
		}[attribute]
		if !ok {
			name = fmt.Sprintf("unknown:%d", attribute)
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func edgeAttributes(attributes []common.EdgeAttribute) []string {
	result := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		name, ok := map[common.EdgeAttribute]string{
			common.IntraAge: "intraAge", common.CrossAge: "crossAge", common.Disabled: "disabled",
			common.SelfReference: "selfReference", common.NotImplemented: "notImplemented", common.Backtracking: "backtracking",
			common.RestrictiveTransitivityTail: "restrictiveTransitivityTail", common.RestrictiveTransitivityHead: "restrictiveTransitivityHead",
		}[attribute]
		if !ok {
			name = fmt.Sprintf("unknown:%d", attribute)
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func addZones(data *graphData, corpus *hypercard.Corpus) error {
	type cardView struct {
		card          hypercard.Card
		background    hypercard.Background
		width, height int
	}
	views := make(map[string]cardView)
	for _, stack := range corpus.Stacks {
		backgrounds := make(map[int]hypercard.Background)
		for _, background := range stack.Backgrounds {
			backgrounds[background.ID] = background
		}
		width, height := stack.Width, stack.Height
		if width <= 0 {
			width = 512
		}
		if height <= 0 {
			height = 342
		}
		for _, card := range stack.Cards {
			key := fmt.Sprintf("%s:%d", stack.Name, card.ID)
			if _, exists := views[key]; exists {
				return fmt.Errorf("HTML zone geometry has duplicate card %q", key)
			}
			views[key] = cardView{card: card, background: backgrounds[card.BackgroundID], width: width, height: height}
		}
	}
	for i := range data.Nodes {
		node := &data.Nodes[i]
		view, exists := views[node.Name]
		if !exists {
			continue
		}
		node.ViewWidth, node.ViewHeight = view.width, view.height
		for _, layer := range []struct {
			name  string
			parts []hypercard.Part
		}{
			{"background", view.background.Parts}, {"card", view.card.Parts},
		} {
			for _, part := range layer.parts {
				width, height := part.Rect.Right-part.Rect.Left, part.Rect.Bottom-part.Rect.Top
				if width <= 0 || height <= 0 {
					continue
				}
				node.Zones = append(node.Zones, zone{ID: part.ID, Type: part.Type, Name: part.Name, Layer: layer.name,
					X: part.Rect.Left, Y: part.Rect.Top, Width: width, Height: height,
					Script: part.Script.Text, Hidden: part.Hidden, Enabled: part.Enabled})
			}
		}
		data.Meta.Zones += len(node.Zones)
	}
	return nil
}
