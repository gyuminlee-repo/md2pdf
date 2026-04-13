# md2pdf

Markdown 파일을 PDF로 변환하는 Windows 독립 실행 프로그램.
설치 없이 `md2pdf.exe` 하나로 실행 가능.

## 사용법

### 드래그앤드롭
`.md` 파일을 `md2pdf.exe`에 끌어다 놓으면 같은 폴더에 `.pdf` 생성.
여러 파일을 한번에 드래그하면 일괄 변환.
GUI 창에 파일을 드래그앤드롭하면 원본 파일의 실제 경로를 인식하여 같은 폴더에 PDF를 생성한다 (OLE IDropTarget COM 구현).

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
- WebView2 GUI + OLE IDropTarget COM (네이티브 드래그앤드롭, 실제 파일 경로 획득)
- Pretendard 폰트 내장 (한글, 유니코드 하첨자/윗첨자/수학 기호 + 자주 쓰는 이모지 글리프 주입)
- D2Coding 코드 폰트 내장 (한글·Mac 심볼 ⌘⌥⌃⇧·box-drawing 모두 지원)
- chroma 기반 코드 구문 강조 (github 테마, Keyword/Error 등 빨간 토큰은 중립화)
- GFM 확장: 테이블, 취소선, 자동 링크, 체크리스트

## 빌드 (WSL/Linux에서 Windows exe 크로스컴파일)

```bash
export PATH=~/go-sdk/go/bin:$PATH
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o md2pdf.exe .
```

또는 `./build.sh` 실행.

## 지원 마크다운 요소

- 제목 (h1-h6), 본문, **굵게**, *기울임*
- 코드 블록 (구문 강조), 인라인 코드
- 테이블, 순서/비순서 리스트, 중첩 리스트
- 인용문, 링크, 수평선
- 이미지 (md 파일 기준 상대경로 자동 해석, 폭은 사용 영역의 1/3로 제한)
- Obsidian `![[파일명.png]]` 임베드: `.obsidian` 폴더 탐색으로 vault 루트 인식 후 vault 전체에서 파일명 검색해 자동 삽입
- Mermaid 코드 블록: kroki.io로 POST 렌더링 → PNG 임베드 (GUI에서 `이미지/캡션 스텁/제거` 선택 가능)
- 헤딩 keep-with-next: 이미지가 다음 페이지로 넘어가면 바로 앞 헤딩도 함께 이동 (사전 페이지 브레이크, 중복 출력 없음)
- CommonMark hard line break (`  \n`, `\\\n`) → 단락 구분으로 변환 (goldmark-pdf가 `\n`을 공백으로 치환하는 문제 우회)
- 한글, 유니코드 하첨자(H₂O), 윗첨자, 수학 기호, 자주 쓰는 이모지(✅❌💡 등) 지원
- 산문의 `->` / `-->` 화살표를 자동으로 `→`로 변환 (코드 블록 내부는 유지)
- YAML frontmatter 자동 제거

## GUI 옵션

테마 옆 드롭다운:
- **테마**: 본문·코드 블록 색 팔레트 선택
- **글씨 크기**: 기본 / 소폭(−10%) / 중간(−17%) / 조밀(−22%) — 헤딩·본문·표·행간 비례 축소
- **Mermaid**: 이미지 렌더링 (기본) / 캡션 스텁 / 제거

## 파일 구조

```
main.go            진입점
converter.go       goldmark → PDF 변환 파이프라인, FontPreset/ConvertOptions 정의
font.go            Pretendard·D2Coding 내장 등록
attachments.go     Obsidian ![[...]] 임베드 해석 (vault 탐색 + 캐시 복사)
mermaid.go         ```mermaid 블록을 kroki.io로 렌더링, 캐시 관리
image_renderer.go  커스텀 Image/Heading 렌더러 (폭 1/3 제한, heading keep-with-next)
gui_windows.go     WebView2 GUI + OLE IDropTarget COM + 테마/크기/Mermaid 드롭다운
gui_other.go       비-Windows CLI 스텁
theme.go           테마 정의
_fonts/            Pretendard-Regular/Bold.ttf, D2Coding-Regular/Bold.ttf (go:embed)
test/sample.md     테스트용 한글 마크다운
```

변환 중 `<md파일_폴더>/_md2pdf_cache/`에 Mermaid/임베드 이미지가 임시 저장되며, 변환 완료 후 자동 삭제됨.
