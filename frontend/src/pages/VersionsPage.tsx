import { Alert, Box, Button, Card, CardContent, Chip, Grid, Stack, Typography } from '@mui/material';
import GitHubIcon from '@mui/icons-material/GitHub';
import LanguageIcon from '@mui/icons-material/Language';
import { useQuery } from '@tanstack/react-query';
import packageJson from '../../package.json';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';

export function VersionsPage() {
  const versions = useQuery({ queryKey: ['softwareVersions'], queryFn: api.softwareVersions });
  const components = [...(versions.data?.components ?? []), ...frontendSoftwareVersions()].map(enrichComponent);

  return (
    <>
      <PageHeader title="Versions" eyebrow="System inventory">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 780 }}>
          Runtime and dependency versions useful for updates, troubleshooting, and diagnostics.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {versions.isError ? <Alert severity="warning">Unable to load backend software versions.</Alert> : null}
        <Grid container spacing={2}>
          {components.map((component) => (
            <Grid key={`${component.source}-${component.name}`} size={{ xs: 12, sm: 6, lg: 4 }}>
              <Card sx={{ height: '100%' }}>
                <CardContent>
                  <Stack spacing={1.25} sx={{ height: '100%' }}>
                    <Stack direction="row" justifyContent="space-between" spacing={1}>
                      <Typography fontWeight={700}>{component.name}</Typography>
                      <Chip label={component.source} size="small" />
                    </Stack>
                    <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-word' }}>
                      {component.version}
                    </Typography>
                    <Typography variant="body2">{component.description}</Typography>
                    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap sx={{ mt: 'auto' }}>
                      {component.website ? (
                        <Button
                          component="a"
                          href={component.website}
                          target="_blank"
                          rel="noreferrer"
                          startIcon={<LanguageIcon />}
                          variant="outlined"
                          size="small"
                        >
                          Site
                        </Button>
                      ) : null}
                      {component.repository ? (
                        <Button
                          component="a"
                          href={component.repository}
                          target="_blank"
                          rel="noreferrer"
                          startIcon={<GitHubIcon />}
                          variant="outlined"
                          size="small"
                        >
                          Source
                        </Button>
                      ) : null}
                    </Stack>
                  </Stack>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      </Box>
    </>
  );
}

function frontendSoftwareVersions() {
  return [
    { name: 'MVForge Frontend', version: packageJson.version, source: 'frontend' },
    { name: 'React', version: packageJson.dependencies.react, source: 'npm' },
    { name: 'Material UI', version: packageJson.dependencies['@mui/material'], source: 'npm' },
    { name: 'TanStack Query', version: packageJson.dependencies['@tanstack/react-query'], source: 'npm' },
    { name: 'Vite', version: packageJson.devDependencies.vite, source: 'npm' },
    { name: 'TypeScript', version: packageJson.devDependencies.typescript, source: 'npm' },
  ];
}

type VersionComponent = {
  name: string;
  version: string;
  source: string;
};

type EnrichedVersionComponent = VersionComponent & {
  description: string;
  website?: string;
  repository?: string;
};

const componentDetails: Record<string, Omit<EnrichedVersionComponent, keyof VersionComponent>> = {
  'MVForge API': {
    description: 'Backend service that exposes the REST API, queue, scanner, advisor, settings, and worker controls.',
  },
  'MVForge Frontend': {
    description: 'The local web interface used to manage libraries, assets, profiles, queue operations, and settings.',
  },
  Go: {
    description: 'Programming language and runtime used to build the MVForge backend service.',
    website: 'https://go.dev/',
    repository: 'https://github.com/golang/go',
  },
  FFmpeg: {
    description: 'Media processing toolkit used for planned and future real video/audio conversions.',
    website: 'https://ffmpeg.org/',
    repository: 'https://git.ffmpeg.org/ffmpeg.git',
  },
  FFprobe: {
    description: 'FFmpeg companion tool used by the scanner to inspect media metadata without converting files.',
    website: 'https://ffmpeg.org/ffprobe.html',
    repository: 'https://git.ffmpeg.org/ffmpeg.git',
  },
  Gin: {
    description: 'HTTP web framework used by the backend API routes and middleware.',
    website: 'https://gin-gonic.com/',
    repository: 'https://github.com/gin-gonic/gin',
  },
  GORM: {
    description: 'Go ORM used by the backend to persist libraries, profiles, queue jobs, scans, and settings.',
    website: 'https://gorm.io/',
    repository: 'https://github.com/go-gorm/gorm',
  },
  'GORM SQLite Driver': {
    description: 'SQLite adapter used by GORM so MVForge can run with a simple local database.',
    website: 'https://gorm.io/docs/connecting_to_the_database.html#SQLite',
    repository: 'https://github.com/go-gorm/sqlite',
  },
  React: {
    description: 'UI library used to build the interactive MVForge frontend.',
    website: 'https://react.dev/',
    repository: 'https://github.com/facebook/react',
  },
  'Material UI': {
    description: 'Component library used for the dark operational interface, tables, dialogs, forms, and controls.',
    website: 'https://mui.com/',
    repository: 'https://github.com/mui/material-ui',
  },
  'TanStack Query': {
    description: 'Data fetching and cache layer used by the frontend to keep API state fresh.',
    website: 'https://tanstack.com/query/latest',
    repository: 'https://github.com/TanStack/query',
  },
  Vite: {
    description: 'Frontend development and build tool used to serve and bundle the React app.',
    website: 'https://vite.dev/',
    repository: 'https://github.com/vitejs/vite',
  },
  TypeScript: {
    description: 'Typed JavaScript language layer used to make frontend changes safer and easier to maintain.',
    website: 'https://www.typescriptlang.org/',
    repository: 'https://github.com/microsoft/TypeScript',
  },
};

function enrichComponent(component: VersionComponent): EnrichedVersionComponent {
  const details = componentDetails[component.name];
  return {
    ...component,
    description: details?.description ?? 'Software component used by MVForge.',
    website: details?.website,
    repository: details?.repository,
  };
}
