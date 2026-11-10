package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/jobqueue"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
)

type jobsService struct{ api *API }

func (s *jobsService) ListJobs(ctx context.Context, req *connect.Request[adminv1.ListJobsRequest]) (*connect.Response[adminv1.ListJobsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	jobs, next, err := s.api.core.ListJobs(ctx, caller.UserID, int(req.Msg.PageSize), req.Msg.PageToken)
	if err != nil {
		return nil, err
	}
	out := make([]*adminv1.Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, jobSummary(job))
	}
	return connect.NewResponse(&adminv1.ListJobsResponse{Jobs: out, NextPageToken: next}), nil
}
func (s *jobsService) GetJob(ctx context.Context, req *connect.Request[adminv1.GetJobRequest]) (*connect.Response[adminv1.GetJobResponse], error) {
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
func (s *jobsService) RetryJob(ctx context.Context, req *connect.Request[adminv1.RetryJobRequest]) (*connect.Response[adminv1.RetryJobResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	job, err := s.api.core.RetryJob(ctx, caller.UserID, req.Msg.Id, req.Msg.RetryVersion)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.RetryJobResponse{Job: jobSummary(job)}), nil
}

// jobSummary is the explicit privacy boundary: internal payload, claim owner,
// dispatch generation, credentials and message references never enter the API.
func jobSummary(job *statev1.JobRecord) *adminv1.Job {
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
	retry := adminv1.RetryJobState_RETRY_JOB_STATE_NOT_FAILED
	if jobqueue.Expired(job, time.Now()) {
		retry = adminv1.RetryJobState_RETRY_JOB_STATE_EXPIRED
	} else if job.Status == jobqueue.Failed {
		retry = adminv1.RetryJobState_RETRY_JOB_STATE_ALLOWED
	}
	return &adminv1.Job{Id: job.Id, Type: job.Type, Status: status, Attempts: job.Attempts, MaxAttempts: job.MaxAttempts, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt, NextAttemptAt: job.NextAttemptAt, Failure: job.Failure, RetryState: &retry, RetryVersion: jobqueue.RetryVersion(job)}
}
