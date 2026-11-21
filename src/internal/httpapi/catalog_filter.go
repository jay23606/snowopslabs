// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	schema "github.com/snowops-oss/snowopslabs/pkg/scenario"
)

// catalogFilter reads ?track= and ?level= from a catalog list request. An
// unknown value is answered with a 400 that names the valid ones, and ok is
// false, so a typo is never mistaken for an empty catalog.
func catalogFilter(w http.ResponseWriter, r *http.Request) (f schema.Filter, ok bool) {
	q := r.URL.Query()
	f, err := schema.ParseFilter(q.Get("track"), q.Get("level"))
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return schema.Filter{}, false
	}
	return f, true
}
