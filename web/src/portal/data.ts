import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@shared/api";
import type { Info, Me } from "./types";

export function useInfo() {
  return useQuery({ queryKey: ["info"], queryFn: () => api<Info>("/info"), staleTime: 60_000 });
}

/** The signed-in account, or null when signed out. */
export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: async () => {
      try {
        return await api<Me>("/me");
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null;
        throw e;
      }
    },
    refetchInterval: 60_000,
  });
}

export function useRefresh() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: ["me"] });
}
