import { useEffect, useState } from "react";

export function useAutoClearedNotice(timeoutMs: number) {
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    if (!notice) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      setNotice(null);
    }, timeoutMs);

    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [notice, timeoutMs]);

  return [notice, setNotice] as const;
}
