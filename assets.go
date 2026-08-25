package main

import "embed"

//go:embed web/*
var assets embed.FS
