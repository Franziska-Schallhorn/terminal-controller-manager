/*
SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors

SPDX-License-Identifier: Apache-2.0
*/

package webhooks

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/gardener/terminal-controller-manager/api/v1alpha1"
	"github.com/gardener/terminal-controller-manager/internal/helpers"
)

// TerminalMutator handles Terminal
type TerminalMutator struct {
	Log logr.Logger

	// Decoder decodes objects
	Decoder admission.Decoder
}

func (h *TerminalMutator) mutatingTerminalFn(t *v1alpha1.Terminal, admissionReq admissionv1.AdmissionRequest) error {
	if t.Annotations == nil {
		t.Annotations = map[string]string{}
	}

	if admissionReq.Operation == admissionv1.Create {
		t.Annotations[v1alpha1.GardenCreatedBy] = admissionReq.UserInfo.Username

		uuid := uuid.NewUUID()

		terminalIdentifier, err := helpers.ToFnvHash(string(uuid))
		if err != nil {
			return err
		}

		t.Spec.Identifier = terminalIdentifier

		h.mutateNamespaceIfTemporary(t, terminalIdentifier)

		t.Annotations[v1alpha1.TerminalLastHeartbeat] = time.Now().UTC().Format(time.RFC3339)
	}

	if t.Annotations[v1alpha1.TerminalOperation] == v1alpha1.TerminalOperationKeepalive {
		delete(t.Annotations, v1alpha1.TerminalOperation)
		t.Annotations[v1alpha1.TerminalLastHeartbeat] = time.Now().UTC().Format(time.RFC3339)
	}

	return nil
}

func (h *TerminalMutator) mutateNamespaceIfTemporary(t *v1alpha1.Terminal, terminalIdentifier string) {
	ns := "term-" + terminalIdentifier

	if ptr.Deref(t.Spec.Host.TemporaryNamespace, false) {
		t.Spec.Host.Namespace = &ns
	}

	if ptr.Deref(t.Spec.Target.TemporaryNamespace, false) {
		t.Spec.Target.Namespace = &ns
	}
}

var _ admission.Handler = &TerminalMutator{}

// Handle handles admission requests.
func (h *TerminalMutator) Handle(_ context.Context, req admission.Request) admission.Response {
	obj := &v1alpha1.Terminal{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	objCopy := obj.DeepCopy()

	err = h.mutatingTerminalFn(objCopy, req.AdmissionRequest)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	marshaledTerminal, err := json.Marshal(objCopy)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledTerminal)
}
