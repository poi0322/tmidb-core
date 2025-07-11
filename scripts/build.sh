#!/bin/bash
# tmiDB 통합 빌드 스크립트

set -e

# 스크립트 디렉터리 설정
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "🔨 tmiDB 통합 빌드 시작..."
echo "📁 프로젝트 루트: $PROJECT_ROOT"

# 환경변수 설정
source "$SCRIPT_DIR/setup-env.sh"

# bin 디렉터리 생성
mkdir -p "$PROJECT_ROOT/bin" "$PROJECT_ROOT/bin"

# 빌드할 컴포넌트 목록
COMPONENTS=(
    "supervisor"
    "api"
    "data-manager"
    "data-consumer"
    "cli"
)

# 전체 빌드 함수
build_all() {
    echo "🔨 모든 컴포넌트 빌드 중..."
    
    for component in "${COMPONENTS[@]}"; do
        build_component "$component"
    done
    
    echo "✅ 모든 컴포넌트 빌드 완료"
}

# 개별 컴포넌트 빌드 함수
build_component() {
    local component=$1
    local component_dir="$PROJECT_ROOT/cmd/$component"
    local output_name="tmidb-$component"
    
    echo "🔨 빌드 중: $component"
    
    if [ ! -d "$component_dir" ]; then
        echo "❌ 컴포넌트 디렉터리를 찾을 수 없음: $component_dir"
        return 1
    fi
    
    cd "$component_dir"
    
    # 개별 빌드 스크립트가 있으면 실행
    if [ -f "build.sh" ]; then
        echo "📜 개별 빌드 스크립트 실행: $component"
        chmod +x build.sh
        ./build.sh
    else
        # 직접 빌드
        echo "🔧 직접 빌드: $component"
        go build -ldflags="-w -s" -o "$PROJECT_ROOT/bin/$output_name" .
        
        if [ $? -eq 0 ]; then
            echo "✅ $component 빌드 완료"
        else
            echo "❌ $component 빌드 실패"
            cd "$PROJECT_ROOT"
            return 1
        fi
    fi
    
    cd "$PROJECT_ROOT"
}

# 클린업 함수
clean() {
    echo "🧹 빌드 아티팩트 정리 중..."
    rm -rf "$PROJECT_ROOT/bin/"*
    rm -rf "$PROJECT_ROOT/bin/"*
    echo "✅ 정리 완료"
}

# 도움말 함수
show_help() {
    echo "tmiDB 빌드 스크립트"
    echo ""
    echo "사용법: $0 [옵션] [컴포넌트]"
    echo ""
    echo "옵션:"
    echo "  all              모든 컴포넌트 빌드"
    echo "  clean            빌드 아티팩트 정리"
    echo "  help             이 도움말 표시"
    echo ""
    echo "컴포넌트:"
    for component in "${COMPONENTS[@]}"; do
        echo "  $component"
    done
    echo ""
    echo "예제:"
    echo "  $0 all           # 모든 컴포넌트 빌드"
    echo "  $0 api           # API 서버만 빌드"
    echo "  $0 clean         # 정리"
}

# 메인 로직
case "${1:-all}" in
    "all")
        build_all
        ;;
    "clean")
        clean
        ;;
    "help"|"-h"|"--help")
        show_help
        ;;
    *)
        if [[ " ${COMPONENTS[*]} " =~ " ${1} " ]]; then
            build_component "$1"
        else
            echo "❌ 알 수 없는 컴포넌트: $1"
            echo "사용 가능한 컴포넌트: ${COMPONENTS[*]}"
            show_help
            exit 1
        fi
        ;;
esac

echo "🎉 빌드 작업 완료!" 