package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type organizationKey struct{}

type Keys struct {
	organizations map[[sha256.Size]byte]string
}

func Load(r io.Reader) (*Keys, error) {
	var entries []struct {
		OrganizationID string `json:"organization_id"`
		SHA256         string `json:"sha256"`
	}
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode key registry: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF || len(entries) == 0 {
		return nil, errors.New("key registry must contain one nonempty array")
	}
	keys := &Keys{organizations: make(map[[sha256.Size]byte]string, len(entries))}
	for i, entry := range entries {
		id := entry.OrganizationID
		if id == "" || len(id) > 128 || strings.TrimSpace(id) != id || strings.ContainsRune(id, 0) {
			return nil, fmt.Errorf("key registry entry %d: invalid organization ID", i+1)
		}
		bytes, err := hex.DecodeString(entry.SHA256)
		if err != nil || len(bytes) != sha256.Size {
			return nil, fmt.Errorf("key registry entry %d: expected SHA-256 digest", i+1)
		}
		digest := [sha256.Size]byte(bytes)
		if _, exists := keys.organizations[digest]; exists {
			return nil, fmt.Errorf("key registry entry %d: duplicate digest", i+1)
		}
		keys.organizations[digest] = id
	}
	return keys, nil
}

func (k *Keys) Authenticate(ctx context.Context, values []string) (context.Context, bool) {
	if k == nil || len(values) != 1 {
		return ctx, false
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || len(token) != 64 {
		return ctx, false
	}
	if _, err := hex.DecodeString(token); err != nil {
		return ctx, false
	}
	org, ok := k.organizations[sha256.Sum256([]byte(token))]
	if !ok {
		return ctx, false
	}
	return context.WithValue(ctx, organizationKey{}, org), true
}

func Organization(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(organizationKey{}).(string)
	return id, ok
}

func (k *Keys) Unary(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	ctx, ok := k.Authenticate(ctx, metadata.ValueFromIncomingContext(ctx, "authorization"))
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "valid bearer key required")
	}
	return next(ctx, request)
}
