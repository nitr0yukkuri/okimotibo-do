-- Keep the current snapshot as the read model while recording accepted
-- recognition transitions in the history table. Heartbeat only changes
-- received_at/expires_at, so it is intentionally not recorded as a status
-- event.
create or replace function public.record_status_event()
returns trigger
language plpgsql
security definer
set search_path = ''
as $$
begin
  if tg_op = 'INSERT' then
    insert into public.status_events (user_id, room_id, status, captured_at)
    values (new.user_id, new.room_id, new.status, new.captured_at);
  elsif old.client_id is distinct from new.client_id
     or old.sequence is distinct from new.sequence
     or old.status is distinct from new.status
     or old.source is distinct from new.source
     or old.hand is distinct from new.hand
     or old.face is distinct from new.face
     or old.captured_at is distinct from new.captured_at then
    insert into public.status_events (user_id, room_id, status, captured_at)
    values (new.user_id, new.room_id, new.status, new.captured_at);
  end if;
  return new;
end;
$$;

drop trigger if exists current_statuses_record_event on public.current_statuses;
create trigger current_statuses_record_event
after insert or update on public.current_statuses
for each row execute function public.record_status_event();

revoke all on function public.record_status_event() from public, anon, authenticated;
grant execute on function public.record_status_event() to service_role;
