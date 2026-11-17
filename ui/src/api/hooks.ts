import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api } from './client';
import type {
  AuditEvent, AuditFilter, Credential, CredentialCreate, CredentialPatch, CredentialSecret, Message,
  MessageDetail, MessageFilter, Overview, Page, Session, Token, TokenCreate, TokenSecret, UpstreamTest,
} from './types';

const PAGE = 50;

export const keys = {
  session: ['session'] as const,
  overview: ['overview'] as const,
  credentials: ['credentials'] as const,
  credential: (id: number) => ['credentials', id] as const,
  messages: (f: MessageFilter) => ['messages', f] as const,
  message: (id: number) => ['messages', 'one', id] as const,
  audit: (f: AuditFilter) => ['audit', f] as const,
  tokens: ['tokens'] as const,
};

export const useSession = () =>
  useQuery({ queryKey: keys.session, queryFn: () => api.get<Session>('/session'), retry: false, staleTime: 60_000 });

export const useOverview = () =>
  useQuery({ queryKey: keys.overview, queryFn: () => api.get<Overview>('/overview'), refetchInterval: 60_000 });

export const useCredentials = () =>
  useQuery({ queryKey: keys.credentials, queryFn: () => api.get<{ items: Credential[] }>('/credentials'), select: (d) => d.items });

export const useCredential = (id: number) =>
  useQuery({ queryKey: keys.credential(id), queryFn: () => api.get<Credential>(`/credentials/${id}`) });

export const useMessages = (filter: MessageFilter) =>
  useInfiniteQuery({
    queryKey: keys.messages(filter),
    queryFn: ({ pageParam }) => api.get<Page<Message>>('/messages', { ...filter, before: pageParam, limit: PAGE }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.next_before ?? undefined,
  });

export const useMessage = (id: number) =>
  useQuery({ queryKey: keys.message(id), queryFn: () => api.get<MessageDetail>(`/messages/${id}`), staleTime: Infinity });

export const useAudit = (filter: AuditFilter) =>
  useInfiniteQuery({
    queryKey: keys.audit(filter),
    queryFn: ({ pageParam }) => api.get<Page<AuditEvent>>('/audit', { ...filter, before: pageParam, limit: PAGE }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.next_before ?? undefined,
  });

export const useTokens = () =>
  useQuery({ queryKey: keys.tokens, queryFn: () => api.get<{ items: Token[] }>('/tokens'), select: (d) => d.items });

// Mutations refresh what they change. None of them retries: a repeated
// create or rotate would be a second action.
export function useCreateCredential() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CredentialCreate) => api.post<CredentialSecret>('/credentials', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.credentials }),
  });
}

export function useUpdateCredential(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CredentialPatch) => api.patch<Credential>(`/credentials/${id}`, body),
    onSuccess: (updated) => {
      qc.setQueryData(keys.credential(id), updated);
      return qc.invalidateQueries({ queryKey: keys.credentials });
    },
  });
}

export function useRotateCredential(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<CredentialSecret>(`/credentials/${id}/rotate`),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.credentials }),
  });
}

export function useDeleteCredential(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.delete(`/credentials/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.credentials }),
  });
}

export function useCreateToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: TokenCreate) => api.post<TokenSecret>('/tokens', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.tokens }),
  });
}

export function useRevokeToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.delete(`/tokens/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.tokens }),
  });
}

export const useTestUpstream = () => useMutation({ mutationFn: () => api.post<UpstreamTest>('/upstream/test') });

export function useRenewCertificate() {
  const qc = useQueryClient();
  return useMutation({
    // No generic here: @typescript-eslint/strict's no-invalid-void-type rejects
    // a bare <void> at a call site (only T<void> type references are allowed).
    // The endpoint returns 202 with no meaningful body, so the inferred result
    // type (unknown) is never read.
    mutationFn: () => api.post('/certificate/renew'),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.overview }),
  });
}

export const useSignOut = () => useMutation({ mutationFn: () => api.delete<{ redirect_url: string }>('/session') });
