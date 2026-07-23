create extension if not exists pgcrypto;

create type public.presence_status as enum ('available', 'neutral', 'busy', 'unknown');
create type public.recognition_source as enum ('hand', 'face', 'none');

create table public.rooms (
  id uuid primary key default gen_random_uuid(),
  name text not null check (char_length(name) between 1 and 100),
  created_by uuid not null references auth.users(id) on delete cascade,
  created_at timestamptz not null default now()
);

create table public.room_members (
  room_id uuid not null references public.rooms(id) on delete cascade,
  user_id uuid not null references auth.users(id) on delete cascade,
  joined_at timestamptz not null default now(),
  primary key (room_id, user_id)
);

create table public.current_statuses (
  user_id uuid not null references auth.users(id) on delete cascade,
  room_id uuid not null references public.rooms(id) on delete cascade,
  client_id text not null check (char_length(client_id) between 1 and 128),
  sequence bigint not null check (sequence > 0),
  status public.presence_status not null,
  source public.recognition_source not null,
  hand jsonb,
  face jsonb,
  captured_at timestamptz not null,
  received_at timestamptz not null default now(),
  expires_at timestamptz not null,
  constraint hand_confidence_valid check (hand is null or ((hand->>'confidence')::numeric between 0 and 1)),
  constraint face_confidence_valid check (face is null or ((face->>'confidence')::numeric between 0 and 1)),
  primary key (room_id, user_id)
);

create table public.status_events (
  id bigint generated always as identity primary key,
  user_id uuid not null references auth.users(id) on delete cascade,
  room_id uuid not null references public.rooms(id) on delete cascade,
  status public.presence_status not null,
  captured_at timestamptz not null,
  created_at timestamptz not null default now()
);

create index status_events_room_time_idx on public.status_events (room_id, created_at desc);
create index current_statuses_expires_at_idx on public.current_statuses (expires_at);

alter table public.rooms enable row level security;
alter table public.room_members enable row level security;
alter table public.current_statuses enable row level security;
alter table public.status_events enable row level security;

create or replace function public.is_room_member(target_room_id uuid)
returns boolean
language sql
stable
security definer
set search_path = ''
as $$
  select exists (
    select 1 from public.room_members
    where room_id = target_room_id and user_id = auth.uid()
  );
$$;

revoke all on function public.is_room_member(uuid) from public;
grant execute on function public.is_room_member(uuid) to authenticated;

create policy "members can read rooms" on public.rooms for select to authenticated
using (public.is_room_member(id));

create policy "members can read membership" on public.room_members for select to authenticated
using (public.is_room_member(room_id));

create policy "members can read current statuses" on public.current_statuses for select to authenticated
using (public.is_room_member(room_id));

create policy "members can read status events" on public.status_events for select to authenticated
using (public.is_room_member(room_id));

revoke insert, update, delete on public.current_statuses from anon, authenticated;
revoke insert, update, delete on public.status_events from anon, authenticated;

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
  where public.current_statuses.captured_at < excluded.captured_at
     or (public.current_statuses.captured_at = excluded.captured_at
         and public.current_statuses.received_at < excluded.received_at);

  get diagnostics affected = row_count;
  return affected = 1;
end;
$$;

revoke all on function public.upsert_current_status(uuid, uuid, text, bigint, public.presence_status, timestamptz, timestamptz, timestamptz, jsonb, jsonb, public.recognition_source) from public, anon, authenticated;
grant execute on function public.upsert_current_status(uuid, uuid, text, bigint, public.presence_status, timestamptz, timestamptz, timestamptz, jsonb, jsonb, public.recognition_source) to service_role;
