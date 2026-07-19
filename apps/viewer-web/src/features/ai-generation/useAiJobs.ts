import { useCallback, useMemo, useState } from "react";
import { controlAiGenerationJob, fetchAiGenerationJobs } from "./api";
import type { AiGenerationJobsResponse } from "./types";
import { getVisibleAiGenerationJobs, partitionAiGenerationJobs, type AiGenerationJobFilter } from "./model";

export function useAiJobs() {
  const [jobs, setJobs] = useState<AiGenerationJobsResponse | null>(null);
  const [jobsError, setJobsError] = useState<string | null>(null);
  const [isJobsLoading, setIsJobsLoading] = useState(false);
  const [jobFilter, setJobFilter] = useState<AiGenerationJobFilter>("active");
  const [controllingJobId, setControllingJobId] = useState<string | null>(null);

  const jobBuckets = useMemo(() => partitionAiGenerationJobs(jobs?.jobs), [jobs]);
  const visibleJobs = useMemo(
    () => getVisibleAiGenerationJobs({ jobs: jobs?.jobs, filter: jobFilter }),
    [jobFilter, jobs]
  );

  const loadJobs = useCallback(async (options?: { background?: boolean }) => {
    const isBackground = options?.background === true;
    if (!isBackground) {
      setIsJobsLoading(true);
    }

    try {
      const nextJobs = await fetchAiGenerationJobs();
      setJobs(nextJobs);
      setJobsError(null);
    } catch (loadError) {
      setJobsError(loadError instanceof Error ? loadError.message : "Unknown error");
    } finally {
      if (!isBackground) {
        setIsJobsLoading(false);
      }
    }
  }, []);

  const controlJob = useCallback(async (novelId: string, jobId: string, action: "pause" | "resume" | "cancel") => {
    setControllingJobId(jobId);
    try {
      await controlAiGenerationJob(novelId, jobId, action);
      await loadJobs({ background: true });
    } catch (controlError) {
      setJobsError(controlError instanceof Error ? controlError.message : "Unknown error");
    } finally {
      setControllingJobId(null);
    }
  }, [loadJobs]);

  return {
    activeJobs: jobBuckets.active,
    completedJobs: jobBuckets.completed,
    controllingJobId,
    controlJob,
    failedJobs: jobBuckets.failed,
    hasJobs: jobs !== null,
    isJobsLoading,
    jobFilter,
    jobs,
    jobsError,
    loadJobs,
    setJobFilter,
    visibleJobs
  };
}
