import React, { createContext, useContext, useEffect, useState } from 'react';
import { UserManager, WebStorageStateStore } from 'oidc-client-ts';
import { oidcConfig } from './authConfig';
import { practitionerApi } from './api/practitionerApi';

const AuthContext = createContext();

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

export const AuthProvider = ({ children }) => {
  const [userManager] = useState(() => {
    return new UserManager({
      ...oidcConfig,
      userStore: new WebStorageStateStore({ store: window.localStorage }),
    });
  });

  const [user, setUser] = useState(null);
  const [isLoading, setIsLoading] = useState(true);
  const [practitionerId, setPractitionerId] = useState(null);
  const ensuringPractitionerRef = React.useRef(false);

  // Function to ensure practitioner exists for the logged-in user
  const ensurePractitioner = async (user) => {
    // Prevent concurrent execution
    if (ensuringPractitionerRef.current) {
      console.log('Practitioner check already in progress, skipping...');
      return;
    }

    ensuringPractitionerRef.current = true;

    try {
      const userId = user?.profile?.sub;
      const userName = user?.profile?.name || user?.profile?.email || 'Unknown User';
      const userEmail = user?.profile?.email;

      if (!userId) {
        console.warn('No user ID found in profile');
        return;
      }

      // If we already have a practitioner ID, skip
      if (practitionerId) {
        console.log('Practitioner ID already set:', practitionerId);
        return;
      }

      // Search for existing practitioner by identifier
      console.log('Searching for Practitioner by identifier:', userId);
      let practitioner = await practitionerApi.searchByIdentifier(userId);

      if (practitioner) {
        // Practitioner found, store the ID in state
        console.log('Practitioner found:', practitioner.id);
        setPractitionerId(practitioner.id);
        return;
      }

      // No practitioner found, create new one
      console.log('No Practitioner found, creating new one for user:', userName);
      const newPractitioner = await practitionerApi.createPractitioner(userId, userName, userEmail);

      // Store the practitioner ID in state
      setPractitionerId(newPractitioner.id);
      console.log('Practitioner created and stored:', newPractitioner.id);
    } catch (err) {
      console.error('Error ensuring practitioner:', err);
    } finally {
      ensuringPractitionerRef.current = false;
    }
  };

  useEffect(() => {
    // Load user on mount
    userManager.getUser().then(async (user) => {
      if (user && !user.expired) {
        setUser(user);
        await ensurePractitioner(user);
      }
      setIsLoading(false);
    });

    // Listen for user loaded event
    const handleUserLoaded = async (user) => {
      setUser(user);
      await ensurePractitioner(user);
      setIsLoading(false);
    };

    // Listen for user unloaded event
    const handleUserUnloaded = () => {
      setUser(null);
    };

    userManager.events.addUserLoaded(handleUserLoaded);
    userManager.events.addUserUnloaded(handleUserUnloaded);

    return () => {
      userManager.events.removeUserLoaded(handleUserLoaded);
      userManager.events.removeUserUnloaded(handleUserUnloaded);
    };
  }, [userManager]);

  const login = () => {
    return userManager.signinRedirect();
  };

  const logout = () => {
    setUser(null);
    setPractitionerId(null);
    return userManager.signoutRedirect();
  };

  const handleCallback = () => {
    return userManager.signinRedirectCallback();
  };

  const value = {
    user,
    isLoading,
    isAuthenticated: !!user && !user.expired,
    practitionerId,
    login,
    logout,
    handleCallback,
    userManager,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

