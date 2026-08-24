-- Pairing credentials must survive API restarts and be claimable only once.
create table public.pairing_grants (
  token text primary key check (char_length(token) between 40 and 128),
  code text not null unique check (char_length(code) = 8),
  room_id text not null check (char_length(room_id) between 1 and 128),
  user_id text not null check (char_length(user_id) between 1 and 128),
  expires_at timestamptz not null,
  claimed_at timestamptz,
  created_at timestamptz not null default now()
);

create index pairing_grants_expires_at_idx on public.pairing_grants (expires_at);
create index pairing_grants_claim_idx on public.pairing_grants (code, claimed_at, expires_at);

alter table public.pairing_grants enable row level security;

-- Only the backend service role can issue, claim, or inspect pairing credentials.
revoke all on public.pairing_grants from public, anon, authenticated;
grant select, insert, update, delete on public.pairing_grants to service_role;
