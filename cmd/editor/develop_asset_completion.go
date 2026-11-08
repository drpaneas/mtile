package main

import (
	"fmt"
	"sort"
	"strings"
)

func (g *Game) openDevelopAssetCompletions() bool {
	context, ok := developAssetContextAt(g.code.Text(), g.code.Cursor())
	if !ok {
		if len(g.codeCompletions) != 0 && g.codeCompletions[0].kind != "" {
			g.codeCompletionOpen = false
		}
		return false
	}
	g.codeCompletions = g.codeCompletions[:0]
	for _, asset := range g.developAssets(context.Kind) {
		if strings.HasPrefix(asset.Name, context.Prefix) {
			g.codeCompletions = append(g.codeCompletions, codeCompletion{name: asset.Name, kind: context.Kind, id: asset.ID})
		}
	}
	sort.SliceStable(g.codeCompletions, func(i, j int) bool { return g.codeCompletions[i].name < g.codeCompletions[j].name })
	g.codeCompletionIndex = 0
	g.codeCompletionOpen = len(g.codeCompletions) > 0
	return true
}

func (g *Game) applyDevelopAssetCompletion(item codeCompletion) {
	context, ok := developAssetContextAt(g.code.Text(), g.code.Cursor())
	if !ok || context.Kind != item.kind || !strings.HasPrefix(item.name, context.Prefix) {
		return
	}
	name := escapeLuaAssetName(item.name, byte(context.Quote))
	if !context.Closed {
		name += string(context.Quote)
	}
	g.code.SetCursor(context.Start, false)
	g.code.SetCursor(context.End, true)
	g.code.Insert(name)
}

func (g *Game) openDevelopAssetAtCaret() {
	context, ok := developAssetContextAt(g.code.Text(), g.code.Cursor())
	if !ok || !context.Closed {
		g.statusMsg = "Place the cursor in a pose or animation name to open it."
		return
	}
	for _, asset := range g.developAssets(context.Kind) {
		if context.Name == asset.Name && g.openDevelopAsset(context.Kind, asset.ID) {
			g.codeCompletionOpen = false
			return
		}
	}
	g.statusMsg = fmt.Sprintf("No asset named %q for %s.", context.Name, context.Kind)
}
