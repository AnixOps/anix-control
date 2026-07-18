package pluginhostsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const DefaultMaxResponseBytes = 1 << 20

type DispatchRequest struct {
	PackageID          string
	PackageVersion     string
	RouteGeneration    uint64
	RequestID          string
	IdempotencyKey     string
	RouteID            string
	Method             string
	RequestBody        []byte
	PrincipalJSON      []byte
	DeadlineUnixMillis int64
}

type DispatchResponse struct {
	StatusCode   uint32
	ResponseBody []byte
	Headers      []Header
	OperationID  string
	FailureCode  string
}

type Header struct {
	Name  string
	Value string
}

type MigrationRequest struct {
	PackageID       string
	PackageVersion  string
	MigrationID     string
	Checkpoint      string
	RouteGeneration uint64
}

type MigrationResponse struct {
	Checkpoint       string
	ValidationDigest string
	Complete         bool
	FailureCode      string
}

type HealthResponse struct {
	Healthy     bool
	LeaseID     string
	DetailsJSON string
}

type DrainResponse struct {
	Drained  bool
	InFlight uint64
}

type Package interface {
	Dispatch(context.Context, DispatchRequest) (DispatchResponse, error)
	Migrate(context.Context, MigrationRequest) (MigrationResponse, error)
	Health(context.Context) (HealthResponse, error)
	Drain(context.Context) (DrainResponse, error)
}

type ServerConfig struct {
	PackageID        string
	PackageVersion   string
	MaxResponseBytes int
}

type Server struct {
	pluginhostv1.UnimplementedControlPackageHostServer

	packageID        string
	packageVersion   string
	maxResponseBytes int
	packageImpl      Package
}

var _ pluginhostv1.ControlPackageHostServer = (*Server)(nil)

func NewServer(config ServerConfig, packageImpl Package) (*Server, error) {
	if config.PackageID == "" {
		return nil, errors.New("plugin host package ID is required")
	}
	if config.PackageVersion == "" {
		return nil, errors.New("plugin host package version is required")
	}
	if packageImpl == nil {
		return nil, errors.New("plugin host package implementation is required")
	}
	if config.MaxResponseBytes < 0 {
		return nil, errors.New("plugin host max response bytes cannot be negative")
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = DefaultMaxResponseBytes
	}

	return &Server{
		packageID:        config.PackageID,
		packageVersion:   config.PackageVersion,
		maxResponseBytes: config.MaxResponseBytes,
		packageImpl:      packageImpl,
	}, nil
}

func (s *Server) Dispatch(ctx context.Context, request *pluginhostv1.DispatchRequest) (*pluginhostv1.DispatchResponse, error) {
	if err := s.validateDispatchRequest(request); err != nil {
		return nil, err
	}

	ctx, cancel := withDeadline(ctx, request.GetDeadlineUnixMillis())
	defer cancel()

	response, err := s.packageImpl.Dispatch(ctx, dispatchRequestFromProto(request))
	if err != nil {
		return nil, packageError("dispatch", err)
	}
	wireResponse := dispatchResponseToProto(response)
	if err := validateDispatchResponse(wireResponse, s.maxResponseBytes); err != nil {
		return nil, err
	}
	return wireResponse, nil
}

func (s *Server) Migrate(ctx context.Context, request *pluginhostv1.MigrationRequest) (*pluginhostv1.MigrationResponse, error) {
	if err := s.validateMigrationRequest(request); err != nil {
		return nil, err
	}

	response, err := s.packageImpl.Migrate(ctx, migrationRequestFromProto(request))
	if err != nil {
		return nil, packageError("migrate", err)
	}
	return migrationResponseToProto(response), nil
}

func (s *Server) Health(ctx context.Context, request *pluginhostv1.HealthRequest) (*pluginhostv1.HealthResponse, error) {
	if err := validateGeneration(request.GetRouteGeneration()); err != nil {
		return nil, err
	}

	response, err := s.packageImpl.Health(ctx)
	if err != nil {
		return nil, packageError("health", err)
	}
	return healthResponseToProto(response), nil
}

func (s *Server) Drain(ctx context.Context, request *pluginhostv1.DrainRequest) (*pluginhostv1.DrainResponse, error) {
	if err := validateDrainRequest(request); err != nil {
		return nil, err
	}

	ctx, cancel := withDeadline(ctx, request.GetDeadlineUnixMillis())
	defer cancel()

	response, err := s.packageImpl.Drain(ctx)
	if err != nil {
		return nil, packageError("drain", err)
	}
	return drainResponseToProto(response), nil
}

func (s *Server) validateDispatchRequest(request *pluginhostv1.DispatchRequest) error {
	if request == nil {
		return status.Error(codes.InvalidArgument, "dispatch request is required")
	}
	if err := s.validatePackage(request.GetPackageId(), request.GetPackageVersion()); err != nil {
		return err
	}
	if err := validateGeneration(request.GetRouteGeneration()); err != nil {
		return err
	}
	if err := validateFutureDeadline(request.GetDeadlineUnixMillis()); err != nil {
		return err
	}
	if !json.Valid(request.GetPrincipalJson()) {
		return status.Error(codes.InvalidArgument, "principal JSON is invalid")
	}
	return nil
}

func (s *Server) validateMigrationRequest(request *pluginhostv1.MigrationRequest) error {
	if request == nil {
		return status.Error(codes.InvalidArgument, "migration request is required")
	}
	if err := s.validatePackage(request.GetPackageId(), request.GetPackageVersion()); err != nil {
		return err
	}
	return validateGeneration(request.GetRouteGeneration())
}

func (s *Server) validatePackage(packageID, packageVersion string) error {
	if packageID != s.packageID || packageVersion != s.packageVersion {
		return status.Error(codes.InvalidArgument, "package identity does not match host")
	}
	return nil
}

func validateGeneration(generation uint64) error {
	if generation == 0 {
		return status.Error(codes.InvalidArgument, "route generation is required")
	}
	return nil
}

func validateDrainRequest(request *pluginhostv1.DrainRequest) error {
	if request == nil {
		return status.Error(codes.InvalidArgument, "drain request is required")
	}
	if err := validateGeneration(request.GetRouteGeneration()); err != nil {
		return err
	}
	return validateFutureDeadline(request.GetDeadlineUnixMillis())
}

func validateFutureDeadline(unixMillis int64) error {
	if !time.UnixMilli(unixMillis).After(time.Now()) {
		return status.Error(codes.DeadlineExceeded, "request deadline has expired")
	}
	return nil
}

func withDeadline(ctx context.Context, unixMillis int64) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, time.UnixMilli(unixMillis))
}

func validateDispatchResponse(response *pluginhostv1.DispatchResponse, maxResponseBytes int) error {
	if response.GetStatusCode() < 100 || response.GetStatusCode() > 599 {
		return status.Error(codes.FailedPrecondition, "package response status code is invalid")
	}
	if proto.Size(response) > maxResponseBytes {
		return status.Error(codes.ResourceExhausted, "package response exceeds host response limit")
	}
	return nil
}

func packageError(operation string, err error) error {
	if status.Code(err) != codes.Unknown {
		return err
	}
	return status.Error(codes.Internal, fmt.Sprintf("package %s failed", operation))
}

func dispatchRequestFromProto(request *pluginhostv1.DispatchRequest) DispatchRequest {
	return DispatchRequest{
		PackageID:          request.GetPackageId(),
		PackageVersion:     request.GetPackageVersion(),
		RouteGeneration:    request.GetRouteGeneration(),
		RequestID:          request.GetRequestId(),
		IdempotencyKey:     request.GetIdempotencyKey(),
		RouteID:            request.GetRouteId(),
		Method:             request.GetMethod(),
		RequestBody:        append([]byte(nil), request.GetRequestBody()...),
		PrincipalJSON:      append([]byte(nil), request.GetPrincipalJson()...),
		DeadlineUnixMillis: request.GetDeadlineUnixMillis(),
	}
}

func dispatchResponseToProto(response DispatchResponse) *pluginhostv1.DispatchResponse {
	headers := make([]*pluginhostv1.Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = &pluginhostv1.Header{Name: header.Name, Value: header.Value}
	}

	return &pluginhostv1.DispatchResponse{
		StatusCode:   response.StatusCode,
		ResponseBody: append([]byte(nil), response.ResponseBody...),
		Headers:      headers,
		OperationId:  response.OperationID,
		FailureCode:  response.FailureCode,
	}
}

func migrationRequestFromProto(request *pluginhostv1.MigrationRequest) MigrationRequest {
	return MigrationRequest{
		PackageID:       request.GetPackageId(),
		PackageVersion:  request.GetPackageVersion(),
		MigrationID:     request.GetMigrationId(),
		Checkpoint:      request.GetCheckpoint(),
		RouteGeneration: request.GetRouteGeneration(),
	}
}

func migrationResponseToProto(response MigrationResponse) *pluginhostv1.MigrationResponse {
	return &pluginhostv1.MigrationResponse{
		Checkpoint:       response.Checkpoint,
		ValidationDigest: response.ValidationDigest,
		Complete:         response.Complete,
		FailureCode:      response.FailureCode,
	}
}

func healthResponseToProto(response HealthResponse) *pluginhostv1.HealthResponse {
	return &pluginhostv1.HealthResponse{
		Healthy:     response.Healthy,
		LeaseId:     response.LeaseID,
		DetailsJson: response.DetailsJSON,
	}
}

func drainResponseToProto(response DrainResponse) *pluginhostv1.DrainResponse {
	return &pluginhostv1.DrainResponse{
		Drained:  response.Drained,
		InFlight: response.InFlight,
	}
}
