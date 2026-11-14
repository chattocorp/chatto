package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

type adminJobService struct{ api *API }

func (s *adminJobService) ListJobs(ctx context.Context, req *connect.Request[adminv1.ListJobsRequest]) (*connect.Response[adminv1.ListJobsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	status := jobqueue.Unknown
	switch req.Msg.Status {
	case adminv1.JobStatus_JOB_STATUS_QUEUED:
		status = jobqueue.Queued
	case adminv1.JobStatus_JOB_STATUS_RUNNING:
		status = jobqueue.Running
	case adminv1.JobStatus_JOB_STATUS_SUCCEEDED:
		status = jobqueue.Succeeded
	case adminv1.JobStatus_JOB_STATUS_FAILED:
		status = jobqueue.Failed
	}
	jobs, next, err := s.api.core.ListJobs(ctx, caller.UserID, int(req.Msg.PageSize), req.Msg.Cursor, status)
	if err != nil {
		return nil, err
	}
	out := make([]*adminv1.Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, jobSummary(job))
	}
	return connect.NewResponse(&adminv1.ListJobsResponse{Jobs: out, NextCursor: next}), nil
}
func (s *adminJobService) GetJob(ctx context.Context, req *connect.Request[adminv1.GetJobRequest]) (*connect.Response[adminv1.GetJobResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	job, err := s.api.core.GetJob(ctx, caller.UserID, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetJobResponse{Job: jobSummary(job)}), nil
}
func (s *adminJobService) RetryJob(ctx context.Context, req *connect.Request[adminv1.RetryJobRequest]) (*connect.Response[adminv1.RetryJobResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	job, err := s.api.core.RetryJob(ctx, caller.UserID, req.Msg.Id, req.Msg.Version)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.RetryJobResponse{Job: jobSummary(job)}), nil
}

func (s *adminJobService) DeleteJob(ctx context.Context, req *connect.Request[adminv1.DeleteJobRequest]) (*connect.Response[adminv1.DeleteJobResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.api.core.DeleteJob(ctx, caller.UserID, req.Msg.Id, req.Msg.Version); err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.DeleteJobResponse{}), nil
}

// jobSummary is the explicit privacy boundary: internal payloads, broker
// positions, credentials, and message references never enter the API.
func jobSummary(job jobqueue.Job) *adminv1.Job {
	status := adminv1.JobStatus_JOB_STATUS_UNSPECIFIED
	switch job.Status {
	case jobqueue.Queued:
		status = adminv1.JobStatus_JOB_STATUS_QUEUED
	case jobqueue.Running:
		status = adminv1.JobStatus_JOB_STATUS_RUNNING
	case jobqueue.Succeeded:
		status = adminv1.JobStatus_JOB_STATUS_SUCCEEDED
	case jobqueue.Failed:
		status = adminv1.JobStatus_JOB_STATUS_FAILED
	}
	// Expiry blocks only the retry of a failed job; other states are not failed.
	retry := adminv1.JobRetryAvailability_JOB_RETRY_AVAILABILITY_NOT_FAILED
	if job.Status == jobqueue.Failed {
		retry = adminv1.JobRetryAvailability_JOB_RETRY_AVAILABILITY_ALLOWED
		if job.Expired(time.Now()) {
			retry = adminv1.JobRetryAvailability_JOB_RETRY_AVAILABILITY_EXPIRED
		}
	}
	// An unlimited budget has no maximum to show.
	var maxAttempts *uint32
	if job.MaxAttempts != jobqueue.UnlimitedAttempts {
		maxAttempts = &job.MaxAttempts
	}
	return &adminv1.Job{Id: job.ID, Type: job.Type, Status: status, Attempts: job.Attempts, MaxAttempts: maxAttempts, CreatedAt: optionalTimestamp(job.CreatedAt), UpdatedAt: optionalTimestamp(job.UpdatedAt), NextAttemptAt: optionalTimestamp(job.NextAttemptAt), Failure: job.Failure, Retry: &retry, Version: jobqueue.Version(job)}
}
