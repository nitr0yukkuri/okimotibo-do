-- Existing STATUS_TTL=0 rows must not survive the new default forever.
update public.current_statuses
set expires_at = now()
where source <> 'manual'
  and expires_at > now() + interval '24 hours';

create or replace function public.upsert_current_status(
  p_user_id uuid,
  p_room_id uuid,
  p_client_id text,
  p_sequence bigint,
  p_status public.presence_status,
  p_captured_at timestamptz,
  p_received_at timestamptz,
  p_expires_at timestamptz,
  p_hand jsonb,
  p_face jsonb,
  p_source public.recognition_source
)
returns boolean
language plpgsql
security definer
set search_path = ''
as $$
declare
  affected integer;
begin
  insert into public.current_statuses (
    user_id, room_id, client_id, sequence, status, source, hand, face, captured_at, received_at, expires_at
  ) values (
    p_user_id, p_room_id, p_client_id, p_sequence, p_status, p_source, p_hand, p_face, p_captured_at, p_received_at, p_expires_at
  )
  on conflict (room_id, user_id) do update set
    client_id = excluded.client_id,
    sequence = excluded.sequence,
    status = excluded.status,
    source = excluded.source,
    hand = excluded.hand,
    face = excluded.face,
    captured_at = excluded.captured_at,
    received_at = excluded.received_at,
    expires_at = excluded.expires_at
  where (
    public.current_statuses.expires_at <= excluded.received_at
    or public.current_statuses.source <> 'manual'
    or excluded.source in ('manual', 'hand')
  )
    and (public.current_statuses.captured_at < excluded.captured_at
      or (public.current_statuses.captured_at = excluded.captured_at
          and public.current_statuses.received_at < excluded.received_at));

  get diagnostics affected = row_count;
  return affected = 1;
end;
$$;

revoke all on function public.upsert_current_status(uuid, uuid, text, bigint, public.presence_status, timestamptz, timestamptz, timestamptz, jsonb, jsonb, public.recognition_source) from public, anon, authenticated;
grant execute on function public.upsert_current_status(uuid, uuid, text, bigint, public.presence_status, timestamptz, timestamptz, timestamptz, jsonb, jsonb, public.recognition_source) to service_role;
