FROM ghcr.io/lesomnus/dev-golang:1.24



USER hypnos

RUN  echo \
		google.golang.org/protobuf/cmd/protoc-gen-go@v1.36 \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5 \
	| xargs -n1 go install

# Comment out the following lines if you don't want to use Zsh.
RUN wget https://github.com/robbyrussell/oh-my-zsh/raw/master/tools/install.sh -O - | zsh || true \
	&& git clone https://github.com/zsh-users/zsh-autosuggestions         ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-autosuggestions     \
	&& git clone https://github.com/zsh-users/zsh-syntax-highlighting.git ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-syntax-highlighting \
	&& git clone https://github.com/zsh-users/zsh-completions             ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-completions
ADD .zshrc /home/hypnos
ENV SHELL=/bin/zsh
