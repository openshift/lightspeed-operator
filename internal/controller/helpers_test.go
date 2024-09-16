package controller

import (
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func statusHasCondition(status olsv1alpha1.OLSConfigStatus, condition metav1.Condition) bool {
	// ignore ObservedGeneration and LastTransitionTime
	for _, c := range status.Conditions {
		if c.Type == condition.Type &&
			c.Status == condition.Status &&
			c.Reason == condition.Reason &&
			c.Message == condition.Message {
			return true
		}
	}
	return false
}
