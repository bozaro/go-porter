pipeline {
    agent {
        docker {
            label 'onspot'
            image 'golang:1.13'
        }
    }

    stages {
        stage("Cleanup") {
            steps {
                sh "git clean -fdxq"
            }
        }

        stage("Vendors") {
            steps {
                withEnv([
                        "HOME=${WORKSPACE}",
                ]) {
                    sh "go mod download"
                }
            }
        }

        stage("Build for all platforms") {
            matrix {
                axes {
                    axis {
                        name "GOOS"
                        values "linux", "darwin"
                    }
                    axis {
                        name "GOARCH"
                        values "amd64"
                    }
                }
                stages {
                    stage('Build') {
                        steps {
                            withEnv([
                                    "HOME=${WORKSPACE}",
                                    "CGO_ENABLED=0",
                            ]) {
                                sh "go build -o ${WORKSPACE}/bin/${GOOS}_${GOARCH}/porter ./cli/..."
                            }
                        }
                    }
                }
            }
        }

        stage("Archive") {
            steps {
                dir("bin") {
                    archiveArtifacts artifacts: "**", fingerprint: false
                }
            }
        }
    }
}
