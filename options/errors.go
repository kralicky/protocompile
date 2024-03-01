package options

import (
	"fmt"

	"github.com/kralicky/protocompile/ast"
	"github.com/kralicky/protocompile/protointernal"
	"github.com/kralicky/protocompile/reporter"
)

type interpreterError struct {
	base error
	mc   *protointernal.MessageContext
	node ast.Node
}

func (e *interpreterError) Error() string {
	return e.base.Error()
}

func (e *interpreterError) Unwrap() error {
	return e.base
}

func (e *interpreterError) Node() ast.Node {
	return e.node
}

// The option could not be found with the given name.
type OptionNotFoundError interface {
	error

	Node() ast.Node
	isOptionNotFoundError()
}

// The option could be found, but is disallowed in the current context.
type OptionForbiddenError interface {
	error

	Node() ast.Node
	isOptionForbiddenError()
}

// The option could be found and is allowed in the current context, but the value
// is not of the expected type.
type OptionTypeMismatchError interface {
	error

	Node() ast.Node
	isOptionTypeMismatchError()
}

// The option could be found and is allowed in the current context, and the value
// is of the expected type, but is otherwise invalid.
type OptionValueError interface {
	error

	Node() ast.Node
	isOptionValueError()
}

type optionNotFoundError struct {
	interpreterError
}

func (e *optionNotFoundError) isOptionNotFoundError() {}

type optionForbiddenError struct {
	interpreterError
}

func (e *optionForbiddenError) isOptionForbiddenError() {}

type optionTypeMismatchError struct {
	interpreterError
}

func (e *optionTypeMismatchError) isOptionTypeMismatchError() {}

type optionValueError struct {
	interpreterError
}

var (
	_ OptionForbiddenError    = (*optionForbiddenError)(nil)
	_ OptionNotFoundError     = (*optionNotFoundError)(nil)
	_ OptionTypeMismatchError = (*optionTypeMismatchError)(nil)
	_ OptionValueError        = (*optionValueError)(nil)
)

func (e *optionValueError) isOptionValueError() {}

func (i *interpreter) HandleTypeMismatchErrorf(mc *protointernal.MessageContext, node ast.Node, formatStr string, args ...any) error {
	if err := i.handler.HandleError(reporter.Error(i.nodeInfo(node), &optionTypeMismatchError{
		interpreterError: interpreterError{
			base: fmt.Errorf(formatStr, args...),
			mc:   mc,
			node: node,
		},
	})); err != nil {
		return err
	}
	return i.handler.Error()
}

func (i *interpreter) HandleOptionForbiddenErrorf(mc *protointernal.MessageContext, node ast.Node, formatStr string, args ...any) error {
	if err := i.handler.HandleError(reporter.Error(i.nodeInfo(node), &optionForbiddenError{
		interpreterError: interpreterError{
			base: fmt.Errorf(formatStr, args...),
			mc:   mc,
			node: node,
		},
	})); err != nil {
		return err
	}
	return i.handler.Error()
}

func (i *interpreter) HandleOptionNotFoundErrorf(mc *protointernal.MessageContext, node ast.Node, formatStr string, args ...any) error {
	if err := i.handler.HandleError(reporter.Error(i.nodeInfo(node), &optionNotFoundError{
		interpreterError: interpreterError{
			base: fmt.Errorf(formatStr, args...),
			mc:   mc,
			node: node,
		},
	})); err != nil {
		return err
	}
	return i.handler.Error()
}

func (i *interpreter) HandleOptionValueErrorf(mc *protointernal.MessageContext, node ast.Node, formatStr string, args ...any) error {
	if err := i.handler.HandleError(reporter.Error(i.nodeInfo(node), &optionValueError{
		interpreterError: interpreterError{
			base: fmt.Errorf(formatStr, args...),
			mc:   mc,
			node: node,
		},
	})); err != nil {
		return err
	}
	return i.handler.Error()
}
