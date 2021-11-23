pipeline {
  agent {
    label 'ubuntu_docker_label'
  }
  stages {
    stage("Lint") {
      steps {
        sh "make fmt && git diff --exit-code"
      }
    }
    stage("Test") {
      steps {
        sh "make test"
      }
    }
    stage("Build") {
      steps {
        withDockerRegistry([credentialsId: "<insert-the-creds-id>", url: ""]) {
          sh "make docker push"
        }
      }
    }
    stage("Push") {
      when {
        branch "master"
      }
      steps {
        withDockerRegistry([credentialsId: "<insert-the-creds-id>", url: ""]) {
          sh "make push IMAGE_VERSION=latest"
        }
      }
    }
    
  }
  post {
    cleanup {
      sh "make clean || true"
      cleanWs()
    }
  }
}
