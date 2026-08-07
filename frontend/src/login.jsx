import React from "react";
import "./login.css";
import { supabase } from "./supabase-client";

// 実際のGoogleログインはSupabase Authに委譲する。
// supabase未設定(.env.local未設定)の場合はボタンを無効化し、原因がわかるようにする。
const Login = () => {
    const handleLogin = () => {
        supabase?.auth.signInWithOAuth({
            provider: "google",
            options: { redirectTo: window.location.origin },
        });
    };

    return (
        <div className="login">
            <div className="login-box">
                <img className="login-logo" src="/okimochi_logo.png" alt="おきもちぼ〜ど" />
                <button className="google-btn" type="button" onClick={handleLogin} disabled={!supabase}>
                    <img src="https://developers.google.com/identity/images/g-logo.png" alt="Google Logo"/>
                    Login with Google
                </button>
                {!supabase && (
                    <p className="login-warning">
                        Supabaseの接続設定(VITE_SUPABASE_URL / VITE_SUPABASE_ANON_KEY)が見つかりません
                    </p>
                )}
            </div>
        </div>
    );
};
export default Login;
