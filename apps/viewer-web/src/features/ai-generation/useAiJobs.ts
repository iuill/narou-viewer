import { useCallback, useMemo, useState } from "react";
import { fetchAiGenerationJobs } from "./api";
import type { AiGenerationJobsResponse } from "./types";
import { getVisibleAiGenerationJobs, partitionAiGenerationJobs, type AiGenerationJobFilter } from "./model";

export function useAiJobs() {
  const [jobs, setJobs] = useState<AiGenerationJobsResponse | null>(null);
  const [jobsError, setJobsError] = useState<string | null>(null);
  const [isJobsLoading, setIsJobsLoading] = useState(false);
  const [jobFilter, setJobFilter] = useState<AiGenerationJobFilter>("active");

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

  return {
    activeJobs: jobBuckets.active,
    completedJobs: jobBuckets.completed,
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
