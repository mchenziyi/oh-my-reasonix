package main

import (
	"os"

	"github.com/mchenziyi/oh-my-reasonix/internal/install"
)

func loadAssetsFromInvocation() (install.Assets, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return install.Assets{}, err
	}
	return install.LoadAssets(cwd)
}
