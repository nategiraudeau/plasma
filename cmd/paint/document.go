package main

import (
	"plasma"
	"plasma/internal/preset"
)

type document = preset.Document
type point = preset.Point

func newDocument() document                            { return preset.New() }
func loadDocument(name string) (document, bool, error) { return preset.Load(name) }
func saveDocument(name string, d document) error       { return preset.Save(name, d) }
func parseColor(value string) (plasma.RGB, error)      { return preset.ParseColor(value) }
