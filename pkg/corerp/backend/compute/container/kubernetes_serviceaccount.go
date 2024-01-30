/*
Copyright 2023 The Radius Authors.

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

package container

import (
	"context"

	"github.com/radius-project/radius/pkg/corerp/backend/compute"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ compute.Component = (*kubernetesServiceAccount)(nil)

type kubernetesServiceAccount struct {
	ServiceAccount *corev1.ServiceAccount
	Role           *rbacv1.Role
	RoleBinding    *rbacv1.RoleBinding
}

func (*kubernetesServiceAccount) Update(resource compute.CoreResource) error {
	panic("unimplemented")
}

func (*kubernetesServiceAccount) Deploy(ctx context.Context) error {
	panic("unimplemented")
}
