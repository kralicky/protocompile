// Copyright 2020-2023 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ast_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/parser"
	"github.com/kralicky/protocompile/reporter"
)

func TestItems(t *testing.T) {
	t.Parallel()
	err := filepath.Walk("../internal/testdata", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "incomplete_fields.proto" {
			return nil // this file has odd contents that don't follow the rules we're testing
		}
		if filepath.Ext(path) == ".proto" {
			t.Run(path, func(t *testing.T) {
				t.Parallel()
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				testItemsSequence(t, path, data)
			})
		}
		return nil
	})
	assert.NoError(t, err) //nolint:testifylint // we want to continue even if err!=nil
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		testItemsSequence(t, "empty", []byte(`
		// this file has no lexical elements, just this one comment
		`))
	})
}

func testItemsSequence(t *testing.T, path string, data []byte) {
	filename := filepath.Base(path)
	root, err := parser.Parse(filename, bytes.NewReader(data), reporter.NewHandler(nil), 0)
	require.NoError(t, err)
	tokens := leavesAsSlice(root)
	require.NoError(t, err)
	// Make sure sequence matches the actual leaves in the tree
	seq := root.Items()
	// Both forwards
	item, ok := seq.First()
	require.True(t, ok)
	checkComments := func(comments ast.Comments) {
		for i := 0; i < comments.Len(); i++ {
			c := comments.Index(i)
			if !c.IsVirtual() && c.VirtualItem().IsValid() {
				require.Equal(t, c.VirtualItem(), item)
				continue
			} else {
				if c.AsItem() != item {
					require.Equal(t, c.AttributedTo(), item)
				}
				item, _ = seq.Next(item)
			}
		}
	}
	for _, astToken := range tokens {
		tokInfo := root.TokenInfo(astToken)
		checkComments(tokInfo.LeadingComments())

		astItem := astToken.AsItem()
		if astItem != item {
			t.Logf("expected %v (%q), got %v (%q)", astItem, root.ItemInfo(astItem).RawText(), item, root.ItemInfo(item).RawText())
			t.Log(root.DebugAnnotated())
		}
		require.Equal(t, astItem, item)
		infoEqual(t, tokInfo, root.ItemInfo(astItem))
		checkComments(tokInfo.TrailingComments())
		item, _ = seq.Next(item)
	}
	// And backwards

	// TODO: the logic here is too complex without some sort of lookahead since
	// we now have the ability to attribute comments to tokens we haven't seen
	// yet when iterating in reverse. However, checking that the sequence matches
	// in both directions still tests for correctness of the implementation.
	forwardItems := make([]ast.Item, 0, len(tokens))
	item, ok = seq.First()
	require.True(t, ok)
	for {
		forwardItems = append(forwardItems, item)
		item, ok = seq.Next(item)
		if !ok {
			break
		}
	}
	backwardItems := make([]ast.Item, 0, len(tokens))
	item, ok = seq.Last()
	require.True(t, ok)
	for {
		backwardItems = append(backwardItems, item)
		item, ok = seq.Previous(item)
		if !ok {
			break
		}
	}
	slices.Reverse(backwardItems)
	require.Equal(t, forwardItems, backwardItems)
}

func infoEqual(t *testing.T, exp, act ast.ItemInfo) {
	assert.Equal(t, act.RawText(), exp.RawText())
	assert.Equal(t, act.Start(), exp.Start(), "item %q", act.RawText())
	assert.Equal(t, act.End(), exp.End(), "item %q", act.RawText())
	assert.Equal(t, act.LeadingWhitespace(), exp.LeadingWhitespace(), "item %q", act.RawText())
}
