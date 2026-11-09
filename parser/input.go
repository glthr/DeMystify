package parser

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/glthr/DeMystify/hypercard"
)

// LoadInput accepts a native stack, a directory of native stacks, or a directory
// containing converted XML stacks. Mixed directories require a narrower path.
func LoadInput(path string) (hypercard.Corpus, error) {
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return hypercard.Corpus{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return hypercard.Corpus{}, err
	}
	if !info.IsDir() {
		stack, err := hypercard.ReadFile(root)
		if err != nil {
			return hypercard.Corpus{}, err
		}
		compatibilityOrder(&stack)
		return hypercard.Corpus{Stacks: []hypercard.Stack{stack}}, nil
	}

	var native []string
	hasXML := false
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if _, err := os.Stat(filepath.Join(path, stackFileName)); err == nil {
				hasXML = true
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			info, err = os.Stat(path)
			if err != nil {
				return err
			}
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		var header [8]byte
		_, readErr := io.ReadFull(file, header[:])
		closeErr := file.Close()
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if readErr == nil && string(header[4:]) == "STAK" {
			native = append(native, path)
		}
		return nil
	})
	if err != nil {
		return hypercard.Corpus{}, err
	}
	if hasXML {
		if len(native) != 0 {
			return hypercard.Corpus{}, fmt.Errorf("%s contains native and XML stacks; select one input directory", path)
		}
		return LoadXML(root)
	}
	if len(native) == 0 {
		return hypercard.Corpus{}, fmt.Errorf("no native HyperCard stacks or %s files found in %s", stackFileName, path)
	}
	var corpus hypercard.Corpus
	for _, path := range native {
		stack, err := hypercard.ReadFile(path)
		if err != nil {
			return hypercard.Corpus{}, err
		}
		compatibilityOrder(&stack)
		corpus.Stacks = append(corpus.Stacks, stack)
	}
	return corpus, nil
}

// The XML importer historically enumerated filenames lexicographically. Keep
// that graph order while Stack.CardOrder retains the native LIST/PAGE order.
func compatibilityOrder(stack *hypercard.Stack) {
	sort.SliceStable(stack.Cards, func(i, j int) bool {
		return fmt.Sprintf("card_%d.xml", stack.Cards[i].ID) < fmt.Sprintf("card_%d.xml", stack.Cards[j].ID)
	})
	sort.SliceStable(stack.Backgrounds, func(i, j int) bool {
		return fmt.Sprintf("background_%d.xml", stack.Backgrounds[i].ID) < fmt.Sprintf("background_%d.xml", stack.Backgrounds[j].ID)
	})
}
