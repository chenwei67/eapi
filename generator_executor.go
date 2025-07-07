package eapi

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chenwei67/eapi/generators"
	"github.com/chenwei67/eapi/spec"
)

type generatorExecutor struct {
	cfg        *GeneratorConfig
	doc        *spec.T
	getConfig  func(key string) interface{}
	strictMode bool
}

func newGeneratorExecutor(cfg *GeneratorConfig, doc *spec.T, getConfig func(key string) interface{}, strictMode bool) *generatorExecutor {
	return &generatorExecutor{cfg: cfg, doc: doc, getConfig: getConfig, strictMode: strictMode}
}

func (r *generatorExecutor) execute() (err error) {
	var ok bool
	item := r.cfg
	var generator *generators.Generator
	if item.File != "" {
		generator = generators.NewGeneratorFromFile(item.File)
	} else {
		if item.Name == "" {
			return fmt.Errorf("generator name or file cannot be empty")
		}
		generator, ok = generators.Generators[item.Name]
		if !ok {
			return fmt.Errorf("generator '%s' not exists", item.Name)
		}
	}

	err = r.generate(generator)
	if err != nil {
		return
	}

	return
}

func (r *generatorExecutor) generate(t *generators.Generator) error {
	errorLogger := func(format string, args ...interface{}) {
		LogStrictError(format, args...)
	}
	result := t.Print(r.doc, &generators.PrintOptions{
		GetConfig:   r.getConfig,
		StrictMode:  r.strictMode,
		ErrorLogger: errorLogger,
	})
	for _, item := range result {
		outputFile := filepath.Join(r.cfg.Output, item.FileName)
		dir := filepath.Dir(outputFile)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("create directory %s error: %w", dir, err)
		}
		file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.ModePerm)
		if err != nil {
			return fmt.Errorf("open file %s error: %w", outputFile, err)
		}
		_, err = file.WriteString(item.Code)
		if err != nil {
			return fmt.Errorf("write file %s error: %w", outputFile, err)
		}
		file.Close()
	}
	return nil
}
