/*
Copyright 2021 The Kubernetes Authors.

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

// Package check implements checks for managed topology.
package check

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectsAreStrictlyCompatible checks if two referenced objects are strictly compatible, meaning that
// they are compatible and the name of the objects do not change.
func ObjectsAreStrictlyCompatible(current, desired client.Object) field.ErrorList {
	var allErrs field.ErrorList
	if current.GetName() != desired.GetName() {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "name"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v",
				current.GetName(), desired.GetName(), current.GetObjectKind().GroupVersionKind().GroupKind().String(), current.GetName()),
		))
	}
	allErrs = append(allErrs, ObjectsAreCompatible(current, desired)...)
	return allErrs
}

// ObjectsAreCompatible checks if two referenced objects are compatible, meaning that
// they are of the same GroupKind and in the same namespace.
func ObjectsAreCompatible(current, desired client.Object) field.ErrorList {
	var allErrs field.ErrorList

	currentGK := current.GetObjectKind().GroupVersionKind().GroupKind()
	desiredGK := desired.GetObjectKind().GroupVersionKind().GroupKind()
	if currentGK.Group != desiredGK.Group {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "apiVersion"),
			fmt.Sprintf("group cannot be changed from %v to %v to prevent incompatible changes in %v/%v",
				currentGK.Group, desiredGK.Group, currentGK.String(), current.GetName()),
		))
	}
	if currentGK.Kind != desiredGK.Kind {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "kind"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v",
				currentGK.Kind, desiredGK.Kind, currentGK.String(), current.GetName()),
		))
	}
	allErrs = append(allErrs, ObjectsAreInTheSameNamespace(current, desired)...)
	return allErrs
}

// ObjectsAreInTheSameNamespace checks if two referenced objects are in the same namespace.
func ObjectsAreInTheSameNamespace(current, desired client.Object) field.ErrorList {
	var allErrs field.ErrorList

	// NOTE: this should never happen (webhooks prevent it), but checking for extra safety.
	if current.GetNamespace() != desired.GetNamespace() {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "namespace"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v", current.GetNamespace(), desired.GetNamespace(), current.GetObjectKind().GroupVersionKind().GroupKind().String(), current.GetName()),
		))
	}
	return allErrs
}

// LocalObjectTemplatesAreCompatible checks if two referenced objects are compatible, meaning that
// they are of the same GroupKind and in the same namespace.
func LocalObjectTemplatesAreCompatible(current, desired clusterv1.LocalObjectTemplate, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	currentGK := current.Ref.GetObjectKind().GroupVersionKind().GroupKind()
	desiredGK := desired.Ref.GetObjectKind().GroupVersionKind().GroupKind()

	if currentGK.Group != desiredGK.Group {
		allErrs = append(allErrs, field.Forbidden(
			pathPrefix.Child("ref", "apiVersion"),
			fmt.Sprintf("group cannot be changed from %v to %v to prevent incompatible changes in %v/%v", currentGK.Group, desiredGK.Group, currentGK.String(), current.Ref.Name),
		))
	}
	if currentGK.Kind != desiredGK.Kind {
		allErrs = append(allErrs, field.Forbidden(
			pathPrefix.Child("ref", "kind"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v", currentGK.Kind, desiredGK.Kind, currentGK.String(), current.Ref.Name),
		))
	}
	allErrs = append(allErrs, LocalObjectTemplatesAreInSameNamespace(current, desired, pathPrefix)...)
	return allErrs
}

// LocalObjectTemplatesAreInSameNamespace checks if two referenced objects are in the same namespace.
func LocalObjectTemplatesAreInSameNamespace(current, desired clusterv1.LocalObjectTemplate, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if current.Ref.Namespace != desired.Ref.Namespace {
		allErrs = append(allErrs, field.Forbidden(
			pathPrefix.Child("ref", "namespace"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v",
				current.Ref.Namespace, desired.Ref.Namespace, current.Ref.GetObjectKind().GroupVersionKind().GroupKind().String(), current.Ref.Name),
		))
	}
	return allErrs
}

// LocalObjectTemplateIsValid ensures the template is in the correct namespace, has no nil references, and has a valid Kind and GroupVersion.
func LocalObjectTemplateIsValid(template *clusterv1.LocalObjectTemplate, namespace string, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// check if ref is not nil.
	if template.Ref == nil {
		return field.ErrorList{field.Invalid(
			pathPrefix.Child("ref"),
			"nil",
			"cannot be nil",
		)}
	}

	// check if a name is provided
	if template.Ref.Name == "" {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "name"),
				template.Ref.Name,
				"cannot be empty",
			),
		)
	}

	// validate if namespace matches the provided namespace
	if namespace != "" && template.Ref.Namespace != namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "namespace"),
				template.Ref.Namespace,
				fmt.Sprintf("must be '%s'", namespace),
			),
		)
	}

	// check if kind is a template
	if len(template.Ref.Kind) <= len(clusterv1.TemplateSuffix) || !strings.HasSuffix(template.Ref.Kind, clusterv1.TemplateSuffix) {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "kind"),
				template.Ref.Kind,
				fmt.Sprintf("kind must be of form '<name>%s'", clusterv1.TemplateSuffix),
			),
		)
	}

	// check if apiVersion is valid
	gv, err := schema.ParseGroupVersion(template.Ref.APIVersion)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "apiVersion"),
				template.Ref.APIVersion,
				fmt.Sprintf("must be a valid apiVersion: %v", err),
			),
		)
	}
	if err == nil && gv.Empty() {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "apiVersion"),
				template.Ref.APIVersion,
				"value cannot be empty",
			),
		)
	}
	return allErrs
}

// ClusterClassesAreCompatible checks the compatibility between new and old versions of a Cluster Class.
// It checks that:
// 1) InfrastructureCluster Templates are compatible.
// 2) ControlPlane Templates are compatible.
// 3) ControlPlane InfrastructureMachineTemplates are compatible.
// 4) MachineDeploymentClasses have not been deleted and are compatible.
func ClusterClassesAreCompatible(current, desired *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	if current == nil {
		return nil
	}

	// Validate InfrastructureClusterTemplate changes desired a compatible way.
	allErrs = append(allErrs, LocalObjectTemplatesAreCompatible(current.Spec.Infrastructure, desired.Spec.Infrastructure,
		field.NewPath("spec", "infrastructure"))...)

	// Validate control plane changes desired a compatible way.
	allErrs = append(allErrs, LocalObjectTemplatesAreCompatible(current.Spec.ControlPlane.LocalObjectTemplate, desired.Spec.ControlPlane.LocalObjectTemplate,
		field.NewPath("spec", "controlPlane"))...)
	if desired.Spec.ControlPlane.MachineInfrastructure != nil && current.Spec.ControlPlane.MachineInfrastructure != nil {
		allErrs = append(allErrs, LocalObjectTemplatesAreCompatible(*current.Spec.ControlPlane.MachineInfrastructure, *desired.Spec.ControlPlane.MachineInfrastructure,
			field.NewPath("spec", "controlPlane", "machineInfrastructure"))...)
	}

	// Validate changes to MachineDeployments.
	allErrs = append(allErrs, MachineDeploymentClassesAreCompatible(current, desired)...)

	return allErrs
}

// MachineDeploymentClassesAreCompatible checks if each MachineDeploymentClass in the new ClusterClass is a compatible change from the previous ClusterClass.
// It checks if:
// 1) Any MachineDeploymentClass has been removed.
// 2) If the MachineDeploymentClass.Template.Infrastructure reference has changed its Group or Kind.
func MachineDeploymentClassesAreCompatible(current, desired *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// Ensure no MachineDeployment class was removed.
	classes := classNamesFromWorkerClass(desired.Spec.Workers)
	for i, oldClass := range current.Spec.Workers.MachineDeployments {
		if !classes.Has(oldClass.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "workers", "machineDeployments").Index(i),
					desired.Spec.Workers.MachineDeployments,
					fmt.Sprintf("The %q MachineDeployment class can't be removed.", oldClass.Class),
				),
			)
		}
	}

	// Ensure previous MachineDeployment class was modified in a compatible way.
	for _, class := range desired.Spec.Workers.MachineDeployments {
		for i, oldClass := range current.Spec.Workers.MachineDeployments {
			if class.Class == oldClass.Class {
				// NOTE: class.Template.Metadata and class.Template.Bootstrap are allowed to change;

				// class.Template.Bootstrap is ensured syntactically correct by LocalObjectTemplateIsValid.

				// Validates class.Template.Infrastructure template changes in a compatible way
				allErrs = append(allErrs, LocalObjectTemplatesAreCompatible(oldClass.Template.Infrastructure, class.Template.Infrastructure,
					field.NewPath("spec", "workers", "machineDeployments").Index(i))...)
			}
		}
	}
	return allErrs
}

// MachineDeploymentClassesAreUnique checks that no two MachineDeploymentClasses in a ClusterClass share a name.
func MachineDeploymentClassesAreUnique(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	classes := sets.NewString()
	for i, class := range clusterClass.Spec.Workers.MachineDeployments {
		if classes.Has(class.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("class"),
					class.Class,
					fmt.Sprintf("MachineDeployment class should be unique. MachineDeployment with class %q is defined more than once.", class.Class),
				),
			)
		}
		classes.Insert(class.Class)
	}
	return allErrs
}

// MachineDeploymentTopologiesAreUniqueAndDefinedInClusterClass checks that each MachineDeploymentTopology name is unique, and each class in use is defined in ClusterClass.spec.Workers.MachineDeployments.
func MachineDeploymentTopologiesAreUniqueAndDefinedInClusterClass(desired *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	if desired.Spec.Topology.Workers == nil {
		return nil
	}
	if len(desired.Spec.Topology.Workers.MachineDeployments) == 0 {
		return nil
	}
	// MachineDeployment clusterClass must be defined in the ClusterClass.
	machineDeploymentClasses := classNamesFromWorkerClass(clusterClass.Spec.Workers)
	names := sets.String{}
	for i, md := range desired.Spec.Topology.Workers.MachineDeployments {
		if !machineDeploymentClasses.Has(md.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i),
					md,
					fmt.Sprintf("MachineDeployment clusterClass must be defined in the ClusterClass. MachineDeployment with clusterClass %q not found in ClusterClass %v.",
						md.Name, clusterClass.Name),
				),
			)
		}
		if names.Has(md.Name) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i),
					md,
					fmt.Sprintf("MachineDeploymentTopology names should be unique. MachineDeploymentTopology with name %q is defined more than once.", md.Name),
				),
			)
		}
		names.Insert(md.Name)
	}
	return allErrs
}

// ClusterClassReferencesAreValid checks that each template reference in the ClusterClass is valid .
func ClusterClassReferencesAreValid(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, LocalObjectTemplateIsValid(&clusterClass.Spec.Infrastructure, clusterClass.Namespace,
		field.NewPath("spec", "infrastructure"))...)
	allErrs = append(allErrs, LocalObjectTemplateIsValid(&clusterClass.Spec.ControlPlane.LocalObjectTemplate, clusterClass.Namespace,
		field.NewPath("spec", "controlPlane"))...)
	if clusterClass.Spec.ControlPlane.MachineInfrastructure != nil {
		allErrs = append(allErrs, LocalObjectTemplateIsValid(clusterClass.Spec.ControlPlane.MachineInfrastructure, clusterClass.Namespace, field.NewPath("spec", "controlPlane", "machineInfrastructure"))...)
	}

	for i, mdc := range clusterClass.Spec.Workers.MachineDeployments {
		allErrs = append(allErrs, LocalObjectTemplateIsValid(&mdc.Template.Bootstrap, clusterClass.Namespace,
			field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("template", "bootstrap"))...)
		allErrs = append(allErrs, LocalObjectTemplateIsValid(&mdc.Template.Infrastructure, clusterClass.Namespace,
			field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("template", "infrastructure"))...)
	}
	return allErrs
}

// classNames returns the set of MachineDeployment class names.
func classNamesFromWorkerClass(w clusterv1.WorkersClass) sets.String {
	classes := sets.NewString()
	for _, class := range w.MachineDeployments {
		classes.Insert(class.Class)
	}
	return classes
}