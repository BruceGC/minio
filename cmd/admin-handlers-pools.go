// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/minio/minio/internal/logger"
)

type listErasurePools struct {
	SetCount     int      `json:"setCount"`
	DrivesPerSet int      `json:"drivesPerSet"`
	CmdLine      string   `json:"cmdline"`
	Info         PoolInfo `json:"info"`
}

func (a adminAPIHandlers) ResumeErasurePool(w http.ResponseWriter, r *http.Request) {
	ctx := newContext(r, w, "ResumeErasurePool")

	defer logger.AuditLog(ctx, w, r, mustGetClaimsFromToken(r))

	// Get current object layer instance.
	objectAPI := newObjectLayerFn()
	if objectAPI == nil {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrServerNotInitialized), r.URL)
		return
	}

	// Legacy args style such as non-ellipses style is not supported with this API.
	if globalEndpoints.Legacy() {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrNotImplemented), r.URL)
		return
	}

	pools, ok := objectAPI.(*erasureServerPools)
	if !ok {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrNotImplemented), r.URL)
		return
	}

	vars := mux.Vars(r)
	v := vars["pool"]

	notfound := true
	for id, ep := range globalEndpoints {
		if ep.CmdLine != v {
			continue
		}
		if err := pools.Resume(r.Context(), id); err != nil {
			writeErrorResponseJSON(ctx, w, toAdminAPIErr(ctx, err), r.URL)
			return
		}
		notfound = false
	}

	// We didn't find any matching pools, invalid input
	if notfound {
		writeErrorResponseJSON(ctx, w, toAdminAPIErr(ctx, fmt.Errorf("input pool %s not found", v)), r.URL)
		return
	}
}

func (a adminAPIHandlers) SuspendErasurePool(w http.ResponseWriter, r *http.Request) {
	ctx := newContext(r, w, "SuspendErasurePool")

	defer logger.AuditLog(ctx, w, r, mustGetClaimsFromToken(r))

	// Get current object layer instance.
	objectAPI := newObjectLayerFn()
	if objectAPI == nil {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrServerNotInitialized), r.URL)
		return
	}

	// Legacy args style such as non-ellipses style is not supported with this API.
	if globalEndpoints.Legacy() {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrNotImplemented), r.URL)
		return
	}

	pools, ok := objectAPI.(*erasureServerPools)
	if !ok {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrNotImplemented), r.URL)
		return
	}

	vars := mux.Vars(r)
	v := vars["pool"]

	notfound := true
	for id, ep := range globalEndpoints {
		if ep.CmdLine != v {
			continue
		}
		if err := pools.Suspend(r.Context(), id); err != nil {
			writeErrorResponseJSON(ctx, w, toAdminAPIErr(ctx, err), r.URL)
			return
		}
		notfound = false
	}

	// We didn't find any matching pools, invalid input
	if notfound {
		writeErrorResponseJSON(ctx, w, toAdminAPIErr(ctx, fmt.Errorf("input pool %s not found", v)), r.URL)
		return
	}
}

func (a adminAPIHandlers) ListErasurePools(w http.ResponseWriter, r *http.Request) {
	ctx := newContext(r, w, "ListErasurePools")

	defer logger.AuditLog(ctx, w, r, mustGetClaimsFromToken(r))

	// Get current object layer instance.
	objectAPI := newObjectLayerFn()
	if objectAPI == nil {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrServerNotInitialized), r.URL)
		return
	}

	// Legacy args style such as non-ellipses style is not supported with this API.
	if globalEndpoints.Legacy() {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrNotImplemented), r.URL)
		return
	}

	pools, ok := objectAPI.(*erasureServerPools)
	if !ok {
		writeErrorResponseJSON(ctx, w, errorCodes.ToAPIErr(ErrNotImplemented), r.URL)
		return
	}

	var liPools = make([]listErasurePools, 0, len(globalEndpoints))
	for id, ep := range globalEndpoints {
		info, err := pools.Info(r.Context(), id)
		if err != nil {
			writeErrorResponseJSON(ctx, w, toAdminAPIErr(ctx, err), r.URL)
			return
		}
		liPools = append(liPools, listErasurePools{
			SetCount:     ep.SetCount,
			DrivesPerSet: ep.DrivesPerSet,
			CmdLine:      ep.CmdLine,
			Info:         info,
		})
	}

	buf, err := json.Marshal(liPools)
	if err != nil {
		writeErrorResponseJSON(ctx, w, toAdminAPIErr(ctx, err), r.URL)
		return
	}

	w.Write(buf)
	w.(http.Flusher).Flush()
}
