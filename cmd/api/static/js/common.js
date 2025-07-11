window.currentOrgId = null;

document.addEventListener("DOMContentLoaded", async () => {
  try {
    const data = await apiCall("/api/session/organization");
    if (data && data.org_id) {
      window.currentOrgId = data.org_id;
      console.log("Current Org ID set to:", window.currentOrgId);
    } else {
      // org_id가 없으면, 사용자가 조직을 선택해야 함을 의미
      // 특정 페이지에서는 조직 선택을 유도할 수 있음
      console.log("No organization selected in session.");
    }
  } catch (error) {
    console.error("Failed to get current organization:", error);
  }

  // 토큰이 없고 세션 쿠키가 있으면 자동으로 토큰 발급 요청
  if (
    !localStorage.getItem("api_token") &&
    document.cookie.split(";").some((c) => c.trim().startsWith("fiber_session="))
  ) {
    try {
      const resp = await fetch("/api/session/token", { credentials: "include" });
      const data = await resp.json();
      if (data.token) {
        localStorage.setItem("api_token", data.token);
        window.location.reload();
      }
    } catch (e) {
      // 무시
    }
  }
});

function switchOrganization(orgId) {
  apiCall("/api/session/organization", {
    method: "POST",
    body: JSON.stringify({ org_id: parseInt(orgId, 10) }),
  })
    .then((response) => {
      if (response) {
        // apiCall은 성공 시 객체를 반환
        window.location.reload();
      } else {
        showToast("Failed to switch organization", "error");
      }
    })
    .catch(() => {
      showToast("Failed to switch organization", "error");
    });
}

// 공통 API 호출 함수
async function apiCall(url, options = {}) {
  // 인증이 필요 없는 엔드포인트 목록
  const noAuthEndpoints = [
    "/login",
    "/logout",
    "/setup",
    "/api/setup/organization",
    "/api/setup/status",
  ];
  const isNoAuth = noAuthEndpoints.some((ep) => url.startsWith(ep));

  const token = localStorage.getItem("api_token");
  // 인증 필요 없는 엔드포인트는 토큰 체크/헤더 생략
  if (!token && !isNoAuth) {
    showToast("API 토큰이 설정되어 있지 않습니다. [API 토큰] 메뉴에서 토큰을 선택하세요.", "error");
    throw new Error("API 토큰이 없습니다.");
  }

  // Organization ID 가져오기
  const orgId = window.currentOrgId || localStorage.getItem("currentOrgId");

  const defaultHeaders = {
    "Content-Type": "application/json",
    ...(token && !isNoAuth ? { Authorization: `Bearer ${token}` } : {}),
    ...(orgId && !isNoAuth ? { "X-Organization-ID": orgId } : {}),
  };
  const defaultOptions = {
    method: "GET",
    headers: defaultHeaders,
    credentials: "include", // 항상 쿠키 포함
  };
  const mergedOptions = {
    ...defaultOptions,
    ...options,
    headers: {
      ...defaultHeaders,
      ...(options.headers || {}),
    },
  };
  // 디버깅: 실제 헤더와 토큰 출력
  console.log("[apiCall] url:", url, "token:", token, "headers:", mergedOptions.headers);
  try {
    const response = await fetch(url, mergedOptions);

    // ADD THIS BLOCK TO HANDLE REDIRECTS
    if (response.redirected) {
      window.location.href = response.url;
      return { success: false, error: { message: "Redirected to login" } }; // Or throw an error
    }

    // 401/403 처리: 세션 만료 또는 권한 없음 → 로그인 페이지로 이동
    if (response.status === 401 || response.status === 403) {
      window.location.href = "/login";
      return {
        success: false,
        error: {
          message: "Authentication required or forbidden",
        },
      };
    }
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error?.message || `HTTP error! status: ${response.status}`);
    }
    return data;
  } catch (error) {
    console.error("API 호출 오류:", error);
    return {
      success: false,
      error: {
        message: error.message || "Unknown API error",
      },
    };
  }
}

// 날짜 포맷 함수
function formatDate(dateString) {
  if (!dateString) return "-";
  const options = {
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  };
  return new Date(dateString).toLocaleDateString("ko-KR", options);
}

// 파일 크기 포맷 함수
function formatFileSize(bytes) {
  if (bytes === 0) return "0 Bytes";
  const k = 1024;
  const sizes = ["Bytes", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

// Toast 메시지 (Alpine.js와 연동)
function showToast(message, type = "success") {
  if (window.Alpine && Alpine.store && Alpine.store("toast")) {
    Alpine.store("toast").show(message, type);
  } else {
    alert(message);
  }
}

// window에 등록
window.apiCall = apiCall;
window.showToast = showToast;
window.formatDate = formatDate;
window.formatFileSize = formatFileSize;
