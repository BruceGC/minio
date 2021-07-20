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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/minio/minio/internal/hash"
	"github.com/tinylib/msgp/msgp"
)

type poolDrainInfo struct {
	StartTime    time.Time `json:"startTime" msg:"st"`
	TotalSize    int64     `json:"totalSize" msg:"tsm"`
	TotalObjects int64     `json:"totalObjects" msg:"to"`
}

// PoolInfo captures pool info
type PoolInfo struct {
	ID         int            `json:"id" msg:"id"`
	LastUpdate time.Time      `json:"lastUpdate" msg:"lu"`
	Drain      *poolDrainInfo `json:"drainInfo,omitempty" msg:"dr"`
	Suspend    bool           `json:"suspend" msg:"sp"`
}

//go:generate msgp -file $GOFILE -unexported
type poolMeta struct {
	Version string     `msg:"v"`
	Pools   []PoolInfo `msg:"pls"`
}

func (p poolMeta) Resume(idx int) bool {
	if p.IsSuspended(idx) {
		p.Pools[idx].Suspend = false
		p.Pools[idx].LastUpdate = time.Now()
		return true
	}
	return false
}

func (p poolMeta) Suspend(idx int) bool {
	if p.IsSuspended(idx) {
		return false
	}
	p.Pools[idx].Suspend = true
	p.Pools[idx].LastUpdate = time.Now()
	return true
}

func (p poolMeta) IsSuspended(idx int) bool {
	return p.Pools[idx].Suspend
}

func (p *poolMeta) load(ctx context.Context, set *erasureSets, npools int) (bool, error) {
	gr, err := set.GetObjectNInfo(ctx, minioMetaBucket, poolMetaName,
		nil, http.Header{}, readLock, ObjectOptions{})
	if err != nil && !isErrObjectNotFound(err) {
		return false, err
	}
	if isErrObjectNotFound(err) {
		return true, nil
	}
	defer gr.Close()

	if err = p.DecodeMsg(msgp.NewReader(gr)); err != nil {
		return false, err
	}

	switch p.Version {
	case poolMetaV1:
	default:
		return false, fmt.Errorf("unexpected pool meta version: %s", p.Version)
	}

	// Total pools cannot reduce upon restart
	if len(p.Pools) > npools {
		return false, fmt.Errorf("unexpected number of pools provided expecting %d, found %d - please check your command line", len(p.Pools), npools)
	}

	return len(p.Pools) != npools, nil
}

func (p poolMeta) Clone() poolMeta {
	meta := poolMeta{
		Version: p.Version,
	}
	meta.Pools = append(meta.Pools, p.Pools...)
	return meta
}

func (p poolMeta) save(ctx context.Context, set *erasureSets) error {
	buf, err := p.MarshalMsg(nil)
	if err != nil {
		return err
	}
	br := bytes.NewReader(buf)
	r, err := hash.NewReader(br, br.Size(), "", "", br.Size())
	if err != nil {
		return err
	}
	_, err = set.PutObject(ctx, minioMetaBucket, poolMetaName,
		NewPutObjReader(r), ObjectOptions{})
	return err
}

const (
	poolMetaName = "pool.meta"
	poolMetaV1   = "1"
)

// Init() initializes pools and saves additional information about them
// in pool.meta, that is eventually used for draining the pool, suspend
// and resume.
func (z *erasureServerPools) Init(ctx context.Context) error {
	meta := poolMeta{}

	update, err := meta.load(ctx, z.serverPools[0], len(z.serverPools))
	if err != nil {
		return err
	}

	// if no update is needed return right away.
	if !update {
		z.poolMeta = meta
		return nil
	}

	meta = poolMeta{}

	// looks like new pool was added we need to update,
	// or this is a fresh installation (or an existing
	// installation)
	meta.Version = "1"
	for idx := range z.serverPools {
		meta.Pools = append(meta.Pools, PoolInfo{
			ID:         idx,
			Suspend:    false,
			LastUpdate: time.Now(),
		})
	}
	if err = meta.save(ctx, z.serverPools[0]); err != nil {
		return err
	}
	z.poolMeta = meta
	return nil
}

// Decomission features
func (z *erasureServerPools) Drain(ctx context.Context, idx int) error {
	return nil
}

func (z *erasureServerPools) Info(ctx context.Context, idx int) (PoolInfo, error) {
	if idx < 0 {
		return PoolInfo{}, errInvalidArgument
	}

	z.poolMetaMutex.RLock()
	defer z.poolMetaMutex.RUnlock()

	if idx+1 > len(z.poolMeta.Pools) {
		return PoolInfo{}, errInvalidArgument
	}

	return z.poolMeta.Pools[idx], nil
}

func (z *erasureServerPools) ReloadPoolMeta(ctx context.Context) (err error) {
	meta := poolMeta{}

	update, err := meta.load(ctx, z.serverPools[0], len(z.serverPools))
	if err != nil {
		return err
	}

	// update means we have reached an incorrect state
	// this cannot happen because we do not support hot
	// add of pools, reject such operations.
	if update {
		return errInvalidArgument
	}

	z.poolMetaMutex.Lock()
	defer z.poolMetaMutex.Unlock()

	z.poolMeta = meta
	return nil
}

func (z *erasureServerPools) Resume(ctx context.Context, idx int) (err error) {
	if idx < 0 {
		return errInvalidArgument
	}

	z.poolMetaMutex.Lock()
	defer z.poolMetaMutex.Unlock()

	meta := z.poolMeta.Clone()
	if meta.Resume(idx) {
		defer func() {
			if err == nil {
				z.poolMeta.Resume(idx)
				globalNotificationSys.ReloadPoolMeta(ctx)
			}
		}()
		return meta.save(ctx, z.serverPools[0])
	}
	return nil
}

func (z *erasureServerPools) Suspend(ctx context.Context, idx int) (err error) {
	if idx < 0 {
		return errInvalidArgument
	}

	z.poolMetaMutex.Lock()
	defer z.poolMetaMutex.Unlock()

	meta := z.poolMeta.Clone()
	if meta.Suspend(idx) {
		defer func() {
			if err == nil {
				z.poolMeta.Suspend(idx)
				globalNotificationSys.ReloadPoolMeta(ctx)
			}
		}()
		return meta.save(ctx, z.serverPools[0])
	}
	return nil
}
