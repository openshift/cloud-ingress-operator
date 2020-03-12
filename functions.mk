# Arguments
# 1 - Channel (the branch name in the 'operator bundle' repo)
# 2 - Bundle github name (eg foo/bar)
# 3 - Automator git push token (for "app" username)
# 4 - Whether or not to remove any versions more recent than deployed hash (true or false)
# 5 - saasherder config github repo name (eg bip/bop)
# 6 - saasherder config path (absolute within repo, eg /name/hive.yaml)
# 7 - relative path to bundle generator python script (eg ./build/generate-operator-bundle.py)
# 8 - Catalog registry quay.io organization name (eg openshift-sre)
# Uses these variables (from project.mk or standard.mk):
# Operator image
# Git hash
# Commit count
# Operator version
define create_push_catalog_image
	set -e ;\
	git clone --branch $(1) "https://app:$(3)@gitlab.cee.redhat.com/$(2).git" bundles-$(1) ;\
	mkdir -p bundles-$(1)/$(OPERATOR_NAME) ;\
	removed_versions="" ;\
	if [[ "$$(echo $(4) | tr [:upper:] [:lower:])" == "true" ]]; then \
		deployed_hash=$$(curl -s 'https://gitlab.cee.redhat.com/$(5)/raw/master/$(6)' | $(CONTAINER_ENGINE) run --rm -i evns/yq -r '.services[]|select(.name="$(OPERATOR_NAME)").hash') ;\
		delete=false ;\
		for bundle_path in $$(find bundles-$(1) -mindepth 2 -maxdepth 2 -type d | grep -v .git | sort -V); do \
			if [[ "$${delete}" == false ]]; then \
				bundle=$$(echo $$bundle_path | cut -d / -f 3-) ;\
				version_hash=$$(echo $$bundle | cut -d - -f 2) ;\
				if [[ $(OPERATOR_VERSION) == "$${version_hash}"* ]]; then \
					delete=true ;\
				fi ;\
			else \
				\rm -rf "$${bundle_path}" ;\
				removed_versions="$$bundle $$removed_versions" ;\
			fi ;\
		done ;\
	fi ;\
	previous_version=$$(find bundles-$(1) -mindepth 2 -maxdepth 2 -type d | grep -v .git | sort -V | tail -n 1| cut -d / -f 3-) ;\
	if [[ -z $$previous_version ]]; then \
		previous_version=__undefined__ ;\
	else \
		previous_version="$(OPERATOR_NAME).v$${previous_version}" ;\
	fi ;\
	python $(7) bundles-$(1)/$(OPERATOR_NAME) $(OPERATOR_NAME) $(OPERATOR_NAMESPACE) $(OPERATOR_VERSION) $(OPERATOR_IMAGE_URI) $(1) true $$previous_version ;\
	new_version=$$(find bundles-$(1) -mindepth 2 -maxdepth 2 -type d | grep -v .git | sort -V | tail -n 1 | cut -d / -f 3-) ;\
	if [[ $(OPERATOR_NAME).v$${new_version} == $$previous_version ]]; then \
		echo "Already built this, so no need to continue" ;\
		exit 0 ;\
	fi ;\
	sed -e "s/!CHANNEL!/$(1)/g" \
			-e "s/!OPERATOR_NAME!/$(OPERATOR_NAME)/g" \
			-e "s/!VERSION!/$${new_version}/g" \
			hack/templates/package.yaml > bundles-$(1)/$(OPERATOR_NAME)/$(OPERATOR_NAME).package.yaml ;\
	cd bundles-$(1) ;\
		git add . ;\
		git commit -m "add version $(COMMIT_NUMBER)-$(CURRENT_COMMIT)" -m "replaces: $$previous_version" -m "removed versions: $$removed_versions" ;\
		git push origin $(1) ;\
	cd .. ;\
	$(CONTAINER_ENGINE) build \
		-f build/Dockerfile.catalog_registry \
		--build-arg=SRC_BUNDLES=$$(find bundles-$(1) -mindepth 1 -maxdepth 1 -type d | grep -v .git) \
		-t quay.io/$(8)/$(OPERATOR_NAME)-registry:$(1)-latest \
		. ;\
	skopeo copy --dest-creds $$QUAY_USER:$$QUAY_TOKEN \
		"docker-daemon:quay.io/$(8)/$(OPERATOR_NAME)-registry:$(1)-latest" \
		"docker://quay.io/$(8)/$(OPERATOR_NAME)-registry:$(1)-latest" ;\
	skopeo copy --dest-creds $$QUAY_USER:$$QUAY_TOKEN \
		"docker-daemon:quay.io/$(8)/$(OPERATOR_NAME)-registry:$(1)-latest" \
		"docker://quay.io/$(8)/$(OPERATOR_NAME)-registry:$(1)-$(CURRENT_COMMIT)"
endef
