import { createClient, type SupabaseClient } from "@supabase/supabase-js";

const url = import.meta.env.VITE_SUPABASE_URL;
const anonKey = import.meta.env.VITE_SUPABASE_ANON_KEY;

// URL/anon keyが未設定でもアプリ自体は起動できるようにundefinedを許容する。
// (ログインボタン側で未設定を検知して無効化する)
export const supabase: SupabaseClient | undefined =
  url && anonKey ? createClient(url, anonKey) : undefined;
