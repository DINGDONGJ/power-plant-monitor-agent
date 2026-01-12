package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// AuthConfig 认证配置
type AuthConfig struct {
	Username       string
	Password       string
	SessionTimeout time.Duration
}

// Session 会话信息
type Session struct {
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// AuthManager 认证管理器
type AuthManager struct {
	config   AuthConfig
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewAuthManager 创建认证管理器
func NewAuthManager(cfg AuthConfig) *AuthManager {
	if cfg.Username == "" {
		cfg.Username = "admin"
	}
	if cfg.Password == "" {
		cfg.Password = "admin123"
	}
	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = 24 * time.Hour
	}

	am := &AuthManager{
		config:   cfg,
		sessions: make(map[string]*Session),
	}

	// 启动过期会话清理
	go am.cleanupExpiredSessions()

	return am
}

// generateToken 生成随机 token
func generateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Login 登录验证
func (am *AuthManager) Login(username, password string) (string, bool) {
	if username == am.config.Username && password == am.config.Password {
		token := generateToken()
		am.mu.Lock()
		am.sessions[token] = &Session{
			Username:  username,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(am.config.SessionTimeout),
		}
		am.mu.Unlock()
		return token, true
	}
	return "", false
}

// ValidateToken 验证 token
func (am *AuthManager) ValidateToken(token string) bool {
	am.mu.RLock()
	session, exists := am.sessions[token]
	am.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(session.ExpiresAt) {
		am.mu.Lock()
		delete(am.sessions, token)
		am.mu.Unlock()
		return false
	}

	return true
}

// Logout 登出
func (am *AuthManager) Logout(token string) {
	am.mu.Lock()
	delete(am.sessions, token)
	am.mu.Unlock()
}

// cleanupExpiredSessions 清理过期会话
func (am *AuthManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		am.mu.Lock()
		now := time.Now()
		for token, session := range am.sessions {
			if now.After(session.ExpiresAt) {
				delete(am.sessions, token)
			}
		}
		am.mu.Unlock()
	}
}

// AuthMiddleware 认证中间件
func (am *AuthManager) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 放行登录相关接口和静态资源
		path := r.URL.Path
		if path == "/api/login" || path == "/login" || path == "/login.html" {
			next.ServeHTTP(w, r)
			return
		}

		// 检查 cookie 中的 token
		cookie, err := r.Cookie("session_token")
		if err != nil || !am.ValidateToken(cookie.Value) {
			// API 请求返回 401
			if len(path) > 4 && path[:5] == "/api/" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			// 页面请求重定向到登录页
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// HandleLogin 处理登录请求
func (am *AuthManager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// 返回登录页面
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(loginPageHTML))
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	token, ok := am.Login(req.Username, req.Password)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "用户名或密码错误"})
		return
	}

	// 设置 cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int(am.config.SessionTimeout.Seconds()),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleLogout 处理登出请求
func (am *AuthManager) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		am.Logout(cookie.Value)
	}

	// 清除 cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "session_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

const loginPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>登录 - 电厂核心软件监视保障系统</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
            background: #0a0a0a;
            color: #00ff00;
            min-height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
        }
        .login-container {
            background: #111;
            border: 1px solid #333;
            padding: 40px;
            width: 100%;
            max-width: 400px;
        }
        .login-header {
            text-align: center;
            margin-bottom: 30px;
        }
        .login-header h1 {
            color: #fff;
            font-size: 18px;
            font-weight: bold;
            margin-bottom: 10px;
        }
        .login-header p {
            color: #666;
            font-size: 12px;
        }
        .form-group {
            margin-bottom: 20px;
        }
        .form-group label {
            display: block;
            color: #888;
            margin-bottom: 8px;
            font-size: 13px;
        }
        .form-group input {
            width: 100%;
            padding: 12px;
            background: #0a0a0a;
            border: 1px solid #333;
            color: #00ff00;
            font-family: inherit;
            font-size: 14px;
        }
        .form-group input:focus {
            outline: none;
            border-color: #00ff00;
        }
        .login-btn {
            width: 100%;
            padding: 12px;
            background: #003300;
            border: 1px solid #00ff00;
            color: #00ff00;
            font-family: inherit;
            font-size: 14px;
            cursor: pointer;
            transition: all 0.3s;
        }
        .login-btn:hover {
            background: #004400;
        }
        .login-btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }
        .error-msg {
            color: #ff4444;
            font-size: 13px;
            margin-top: 15px;
            text-align: center;
            display: none;
        }
        .error-msg.show {
            display: block;
        }
        .footer {
            text-align: center;
            margin-top: 20px;
            color: #444;
            font-size: 11px;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <div class="login-header">
            <h1>[ 电厂核心软件监视保障系统 ]</h1>
            <p>Process Monitor Agent v1.0</p>
        </div>
        <form id="loginForm">
            <div class="form-group">
                <label>用户名</label>
                <input type="text" id="username" name="username" autocomplete="username" required>
            </div>
            <div class="form-group">
                <label>密码</label>
                <input type="password" id="password" name="password" autocomplete="current-password" required>
            </div>
            <button type="submit" class="login-btn" id="loginBtn">登 录</button>
            <div class="error-msg" id="errorMsg"></div>
        </form>
        <div class="footer">安全登录 · 会话有效期 24 小时</div>
    </div>
    <script>
        document.getElementById('loginForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const btn = document.getElementById('loginBtn');
            const errorMsg = document.getElementById('errorMsg');
            
            btn.disabled = true;
            btn.textContent = '登录中...';
            errorMsg.classList.remove('show');
            
            try {
                const res = await fetch('/api/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        username: document.getElementById('username').value,
                        password: document.getElementById('password').value
                    })
                });
                
                const data = await res.json();
                
                if (res.ok) {
                    window.location.href = '/';
                } else {
                    errorMsg.textContent = data.error || '登录失败';
                    errorMsg.classList.add('show');
                }
            } catch (err) {
                errorMsg.textContent = '网络错误，请重试';
                errorMsg.classList.add('show');
            } finally {
                btn.disabled = false;
                btn.textContent = '登 录';
            }
        });
        
        // 自动聚焦用户名输入框
        document.getElementById('username').focus();
    </script>
</body>
</html>`
