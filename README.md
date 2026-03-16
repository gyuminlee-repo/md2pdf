# md2pdf

Markdown 파일을 PDF로 변환하는 Windows 독립 실행 프로그램.
설치 없이 `md2pdf.exe` 하나로 실행 가능.

## 사용법

### 드래그앤드롭
`.md` 파일을 `md2pdf.exe`에 끌어다 놓으면 같은 폴더에 `.pdf` 생성.
여러 파일을 한번에 드래그하면 일괄 변환.

### CLI
```
md2pdf.exe input.md                  # 같은 폴더에 input.pdf 생성
md2pdf.exe -o output.pdf input.md    # 출력 경로 지정
md2pdf.exe file1.md file2.md         # 일괄 변환
```

### 인터랙티브
`md2pdf.exe` 더블클릭 → 파일 경로 입력 → 변환.

## 기술 스택

- Go 1.23 + goldmark (CommonMark 파서) + goldmark-pdf (PDF 렌더러)
- Malgun Gothic (Windows 기본 한글 폰트) 자동 탐색
- chroma 기반 코드 구문 강조 (github 테마)
- GFM 확장: 테이블, 취소선, 자동 링크, 체크리스트

## 빌드 (WSL/Linux에서 Windows exe 크로스컴파일)

```bash
export PATH=~/go-sdk/go/bin:$PATH
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o md2pdf.exe .
```

또는 `./build.sh` 실행.

## 지원 마크다운 요소

- 제목 (h1-h6), 본문, **굵게**, *기울임*
- 코드 블록 (구문 강조), 인라인 코드
- 테이블, 순서/비순서 리스트, 중첩 리스트
- 인용문, 링크, 수평선
- 이미지 (md 파일 기준 상대경로 자동 해석)
- 한글 완벽 지원

## 파일 구조

```
main.go         진입점 (CLI/GUI 분기, 다중 파일 처리)
converter.go    goldmark → PDF 변환 파이프라인
font.go         Windows/WSL 시스템 폰트 탐색 및 등록
gui.go          인터랙티브 모드
build.sh        WSL 크로스컴파일 스크립트
test/sample.md  테스트용 한글 마크다운
```
