import "./login.css";

interface LoginProps {
  onLogin: () => void;
}

const Login = ({ onLogin }: LoginProps) => {
  return (
    <div className="login">
      <div className="login-box">
        <img className="login-logo" src="/okimochi_logo.png" alt="おきもちぼ〜ど" />
        <button className="google-btn" type="button" onClick={onLogin}>
          <img src="https://developers.google.com/identity/images/g-logo.png" alt="Google Logo" />
          Login with Google
        </button>
      </div>
    </div>
  );
};

export default Login;
