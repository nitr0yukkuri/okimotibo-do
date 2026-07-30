import "./login.css";

const Login = () => {
    return (
        <div className="login">
            <div className="login-box">
                <h1>おきもちぼ〜ど</h1>
                <button className="google-btn">
                    <img src="https://developers.google.com/identity/images/g-logo.png" alt="Google Logo"/>
                    Login with Google
                </button>
            </div>
        </div>
    );
};
export default Login;