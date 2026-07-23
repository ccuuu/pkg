/*
Copyright 2026 The AlaudaDevops Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package namespace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

var (
	// ErrNamespaceMembershipNotFound identifies an owner Namespace without membership metadata.
	ErrNamespaceMembershipNotFound = errors.New("namespace membership not found")
	// ErrNamespaceMembershipUnavailable identifies a resolver that cannot read Namespace state.
	ErrNamespaceMembershipUnavailable = errors.New("namespace membership unavailable")
)

// NamespaceMembershipResolver resolves the logical Namespace membership of an owner Namespace.
type NamespaceMembershipResolver interface {
	// Resolve returns the membership identity and active Namespace names for ownerNamespace.
	Resolve(ctx context.Context, ownerNamespace string) (NamespaceMembership, error)
}

// NamespaceMembership describes one logical Namespace membership scope.
type NamespaceMembership struct {
	// Identity identifies the logical membership, such as a project name.
	Identity string
	// Namespaces contains sorted, deduplicated, active Namespace names.
	Namespaces []string
}

// NamespaceLister reads Namespace objects using Kubernetes label selectors.
type NamespaceLister interface {
	// Get returns one Namespace by name.
	Get(name string) (*corev1.Namespace, error)
	// List returns Namespaces matching selector.
	List(selector labels.Selector) ([]*corev1.Namespace, error)
}

// LabelNamespaceMembershipResolver resolves membership from one Namespace label.
type LabelNamespaceMembershipResolver struct {
	// Lister reads Namespace objects from an informer cache or an adapter.
	Lister NamespaceLister
	// LabelKey identifies the label that stores the membership identity.
	LabelKey string
}

// NewLabelNamespaceMembershipResolver creates a label-based membership resolver.
func NewLabelNamespaceMembershipResolver(lister NamespaceLister, labelKey string) *LabelNamespaceMembershipResolver {
	return &LabelNamespaceMembershipResolver{
		Lister:   lister,
		LabelKey: strings.TrimSpace(labelKey),
	}
}

// Resolve returns all active Namespaces whose label matches the owner Namespace.
func (r *LabelNamespaceMembershipResolver) Resolve(ctx context.Context, ownerNamespace string) (NamespaceMembership, error) {
	if err := contextError(ctx); err != nil {
		return NamespaceMembership{}, err
	}
	if r == nil || r.Lister == nil {
		return NamespaceMembership{}, fmt.Errorf("%w: Namespace lister is nil", ErrNamespaceMembershipUnavailable)
	}
	if r.LabelKey == "" {
		return NamespaceMembership{}, fmt.Errorf("%w: membership label key is empty", ErrNamespaceMembershipUnavailable)
	}
	ownerNamespace = strings.TrimSpace(ownerNamespace)
	if ownerNamespace == "" {
		return NamespaceMembership{}, fmt.Errorf("%w: owner Namespace is empty", ErrNamespaceMembershipNotFound)
	}

	owner, err := r.Lister.Get(ownerNamespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return NamespaceMembership{}, fmt.Errorf("%w: get owner Namespace %q: %w", ErrNamespaceMembershipNotFound, ownerNamespace, err)
		}
		return NamespaceMembership{}, fmt.Errorf("%w: get owner Namespace %q: %w", ErrNamespaceMembershipUnavailable, ownerNamespace, err)
	}
	if owner == nil || !owner.DeletionTimestamp.IsZero() {
		return NamespaceMembership{}, fmt.Errorf("%w: owner Namespace %q is not active", ErrNamespaceMembershipNotFound, ownerNamespace)
	}
	identity := strings.TrimSpace(owner.Labels[r.LabelKey])
	if identity == "" {
		return NamespaceMembership{}, fmt.Errorf("%w: owner Namespace %q has no label %q", ErrNamespaceMembershipNotFound, ownerNamespace, r.LabelKey)
	}

	selector := labels.Set{r.LabelKey: identity}.AsSelector()
	items, err := r.Lister.List(selector)
	if err != nil {
		return NamespaceMembership{}, fmt.Errorf("%w: list Namespaces for %q: %v", ErrNamespaceMembershipUnavailable, identity, err)
	}
	if err := contextError(ctx); err != nil {
		return NamespaceMembership{}, err
	}

	// The owner was read independently and is part of the membership even if a
	// cache implementation observes the Get and List operations at different moments.
	items = append(append([]*corev1.Namespace(nil), items...), owner)
	names := activeNamespaceNames(items)
	return NamespaceMembership{Identity: identity, Namespaces: names}, nil
}

// contextError returns a stable error when the resolver context is unavailable or cancelled.
func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is nil", ErrNamespaceMembershipUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// activeNamespaceNames returns sorted, deduplicated names for active Namespace objects.
func activeNamespaceNames(items []*corev1.Namespace) []string {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == nil || !item.DeletionTimestamp.IsZero() {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
