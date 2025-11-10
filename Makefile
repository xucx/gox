
PROJECT = gox
TIME = $(shell date '+%Y/%m/%d-%H:%M:%S')

ifeq ($(COMMIT),)
COMMIT := ${shell git rev-parse --short HEAD}
endif

ifeq ($(VERSION),)
	CHANGES = $(shell git status --porcelain --untracked-files=no)
	ifneq ($(CHANGES),)
	    DIRTY = -dirty
	endif

	VERSION = $(COMMIT)$(DIRTY)

	GIT_TAG = $(shell git tag -l --contains HEAD | head -n 1)
	# Override VERSION with the Git tag if the current HEAD has a tag pointing to
	# it AND the worktree isn't dirty.
	ifneq ($(GIT_TAG),)
	    ifeq ($(DIRTY),)
	        VERSION = $(GIT_TAG)
	    endif
	endif
endif


DIST = $(CURDIR)/dist
EXE = ${PROJECT}
BUILD_OPTS = -ldflags="-s -w -X github.com/xucx/gox/internal/version.Version=$(VERSION) \
	-X github.com/xucx/gox/internal/version.BuildRevision=$(COMMIT) \
	-X github.com/xucx/gox/internal/version.BuildTimestamp=$(TIME)"

.PHONY: all
all: build 

.PHONY: build
build: 
	go build -v -o ${DIST}/${EXE} ${BUILD_OPTS} $(CURDIR)

PLATFORMS := linux/amd64 windows/amd64 darwin/amd64
.PHONY: build_all $(PLATFORMS)
build_all: $(PLATFORMS)
$(PLATFORMS):
	$(eval os := $(word 1,$(subst /, ,$@)))
	$(eval arch := $(word 2,$(subst /, ,$@)))
	$(eval suffix := $(if $(filter windows,$(os)),.exe))

	GOOS=$(os) GOARCH=$(arch) go build -o ${DIST}/${EXE}-$(os)-$(arch)${suffix} ${BUILD_OPTS} $(CURDIR)

.PHONY: docker
docker:
	docker build --build-arg="BUILD_VERSION=${VERSION}" --build-arg="BUILD_GITCOMMIT=${COMMIT}" -t ${PROJECT}:$(VERSION) .