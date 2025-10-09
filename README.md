# Lucipedia

https://lucipedia.lenn.rocks

Lucipedia is a continuously generated encyclopedia where every page is AI generated.

Whenever you open a page, Lucipedia first checks whether the article already exists. If it doesn't, mistralai/mistral-small-3.2-24b-instruct writes it on the fly and saves it.

## Deployment

The project is deployable as a whole via various docker compose configurations.

- Use `docker-compose-dev.yml` for local testing.
- Use `docker-compose-prod.yml` for production deployment via Portainer.

### Deployment Notes

The following notes are mostly for myself to remember how to deploy the application.

#### Infrastructure

Deploy stack via Portainer with the following settings:
![Portainer deployment stack settings](assets/portainer_stack_settings.png)

Inject environment variables via Portainer GUI. See the [.env.example](.env.example).

Deployed environment variables are saved in my password manager.

##### Updown Healthcheck

- Add health check in [updown.io](https://updown.io/)


#### CI/CD

The project rebuilds and deploys to different environments via [Github Actions](.github/workflows/rebuild-prod-environment.yml).

See the workflows for the secrets needed in Github.
