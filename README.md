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

## 출력 이름과 안전한 저장

- GUI와 비-Windows CLI의 자동 출력 이름은 기존 파일을 덮어쓰지 않는다. 한 파일만 변환하거나 같은 작업을 다시 실행해도 동일한 보존 정책을 적용한다.
- 변환 전에 전체 목록의 출력 경로를 검사한다. 목록 순서대로 `report.pdf`, `report (2).pdf`, `report (3).pdf`처럼 비어 있는 이름을 배정한다. 이미 존재하는 PDF·디렉터리·심볼릭 링크는 건너뛰며, 파일 이름의 대소문자만 다른 경우도 충돌로 취급한다.
- 목록에 `report (2).md`도 있다면 그 파일의 원래 출력 이름을 먼저 예약한다. 따라서 `a/report.md`, `b/report.md`, `report (2).md`는 각각 `report.pdf`, `report (3).pdf`, `report (2).pdf`가 된다. 실패한 파일에 배정한 번호도 작업 도중 재사용하지 않는다.
- GUI는 이름 변경 개수와 파일별 실제 저장 경로 또는 실패 이유를 표시한다. CLI도 실제 저장된 경로를 출력한다. 변환 중에는 출력 폴더를 바꿀 수 없다.
- PDF를 같은 폴더의 임시 파일에 모두 기록하고 동기화·닫기를 마친 뒤 최종 이름으로 게시한다. 사전 검사 후 다른 프로그램이 그 이름으로 파일을 만들면 해당 변환만 오류로 보고하고 새 파일을 보존한다. 쓰기 실패 시 기존 PDF와 앞서 성공한 결과를 유지하며, 임시 파일은 정리한다.
- 새 PDF는 임시 파일의 비공개 권한(Unix 기준 `0600`, umask가 추가 제한)을 유지한다. 명시적으로 기존 파일을 교체할 때만 기존 파일의 권한을 보존한다.
- 원본 위치에 저장할 때 특정 입력의 폴더가 없거나 접근할 수 없으면 그 파일만 실패 처리한다. 별도로 선택한 공통 출력 폴더를 확인할 수 없으면 전체 변환을 시작하지 않는다.
- 명시적 출력 경로를 받는 Go 함수 `ConvertFile`은 기존 일반 파일의 교체를 계속 지원한다. 입력과 같은 파일을 가리키는 출력은 여전히 거부하며, 출력 심볼릭 링크·디렉터리는 교체하지 않는다.

Windows는 동일 폴더에서 `MoveFileEx`를 사용하고, 비-Windows의 새 파일 게시는 하드 링크를 사용한다. 하드 링크를 지원하지 않는 비-Windows 파일시스템은 덮어쓰기 대신 오류로 종료한다. 강제 종료·전원 차단 시 임시 파일이 남을 수 있으며, 모든 네트워크 파일시스템의 장애 복구 내구성까지 보장하지는 않는다.

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

변환 중 `<md파일_폴더>/_md2pdf_cache-<임의값>/`에 Mermaid/임베드 이미지가 임시 저장되며, 변환 종료 시 해당 변환이 만든 디렉터리만 삭제됨. 기존 `_md2pdf_cache/` 디렉터리와 다른 변환의 캐시는 보존됨. 입력과 출력이 동일한 파일(심볼릭 링크/하드 링크 포함)을 가리키면 원본 보호를 위해 오류를 반환함.

## 테스트

```bash
go test ./...
go test -race ./...  # 지원 플랫폼에서 실행
go vet ./...
```

테스트는 합성 Markdown/이미지와 메모리 HTTP 응답만 사용하며 외부 네트워크를 차단함. 실제 Kroki 서비스, Windows WebView2 GUI 및 OLE 드래그앤드롭 동작은 별도 수동 검증이 필요함.

## 라이선스

코드는 [MIT License](LICENSE) 를 따릅니다.

실행 파일에 내장되는 글꼴은 MIT 가 아니라 각 글꼴의 SIL Open Font License 1.1 을 따릅니다.

| 글꼴 | 저작권 | 라이선스 원문 |
|---|---|---|
| [Pretendard](https://github.com/orioncactus/pretendard) | Kil Hyung-jin | [`_fonts/OFL-Pretendard.txt`](_fonts/OFL-Pretendard.txt) |
| [D2Coding](https://github.com/naver/d2codingfont) | NAVER Corporation | [`_fonts/OFL-D2Coding.txt`](_fonts/OFL-D2Coding.txt) |
