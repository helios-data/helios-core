package core

import (
	"fmt"
	"strings"

	configpb "helios/generated/config"
	ct "helios/internal/component_tree"
	"helios/internal/logger"
)

func BuildComponentTree(cfg *configpb.ComponentTree) (*ct.ComponentTree, error) {
	logger.Debugw("Building component tree", "config", cfg)
	return componentTreeFromProto(cfg)
}

func componentTreeFromProto(cfg *configpb.ComponentTree) (*ct.ComponentTree, error) {
	tree := ct.New()

	if cfg == nil || cfg.GetRoot() == nil {
		return tree, nil
	}

	// The root is special because it has no parent branch.
	if _, err := rootNodeFromProto(tree, cfg.GetRoot()); err != nil {
		return nil, err
	}
	return tree, nil
}

func rootNodeFromProto(tree *ct.ComponentTree, node *configpb.BaseComponent) (string, error) {
	if node == nil {
		return "", fmt.Errorf("component node is nil")
	}

	name := strings.TrimSpace(node.GetName())
	if name == "" {
		return "", fmt.Errorf("component node has empty name")
	}

	switch typed := node.GetNodeType().(type) {
	case *configpb.BaseComponent_Branch:
		branchAddress, err := tree.AddRootComponentGroup(name)
		if err != nil {
			return "", err
		}
		// Once the branch exists, children are attached through tree primitives only.
		for _, child := range typed.Branch.GetChildren() {
			if _, err := nodeFromProto(tree, child, branchAddress); err != nil {
				return "", err
			}
		}
		return branchAddress, nil
	case *configpb.BaseComponent_Leaf:
		component, err := componentFromProto(typed.Leaf)
		if err != nil {
			return "", err
		}
		return tree.AddRootComponent(name, component)
	default:
		return "", fmt.Errorf("component node %q has no node type", name)
	}
}

func nodeFromProto(tree *ct.ComponentTree, node *configpb.BaseComponent, parentAddress string) (string, error) {
	if node == nil {
		return "", fmt.Errorf("component node is nil")
	}

	name := strings.TrimSpace(node.GetName())
	if name == "" {
		return "", fmt.Errorf("component node has empty name")
	}

	switch typed := node.GetNodeType().(type) {
	case *configpb.BaseComponent_Branch:
		branchAddress, err := tree.AddComponentGroup(parentAddress, name)
		if err != nil {
			return "", err
		}
		// Parent validation stays inside the tree package.
		for _, child := range typed.Branch.GetChildren() {
			if _, err := nodeFromProto(tree, child, branchAddress); err != nil {
				return "", err
			}
		}
		return branchAddress, nil
	case *configpb.BaseComponent_Leaf:
		component, err := componentFromProto(typed.Leaf)
		if err != nil {
			return "", err
		}
		return tree.AddComponent(parentAddress, name, component)
	default:
		return "", fmt.Errorf("component node %q has no node type", name)
	}
}

func componentFromProto(leaf *configpb.Component) (*ct.Component, error) {
	if leaf == nil {
		return nil, fmt.Errorf("leaf component is nil")
	}
	// Identity stays on tree nodes; component data is runtime/config state only.
	return ct.NewComponent(leaf.DockerSpec, leaf.GetFlags()), nil
}
